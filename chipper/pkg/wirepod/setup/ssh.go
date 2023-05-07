package botsetup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/kirillgrishin-tech/chipper/pkg/logger"
	"github.com/kirillgrishin-tech/chipper/pkg/vars"
	"golang.org/x/crypto/ssh"
)

// this file will be copied to the bot
const SetupScriptPath = "../vector-cloud/pod-bot-install.sh"

// path to copy to
const BotSetupPath = "/data/pod-bot-install.sh"

var SetupSSHStatus string = "not running"
var SSHSettingUp bool = false

func doErr(err error) error {
	SSHSettingUp = false
	SetupSSHStatus = "not running (last error: " + err.Error() + ")"
	return err
}

func runCmd(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	output, err := session.Output(cmd)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func SetupBotViaSSH(ip string, key []byte) error {
	if !SSHSettingUp {
		logger.Println("Setting up " + ip + " via SSH")
		SetupSSHStatus = "Setting up SSH connection..."
		CreateServerConfig()
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			doErr(err)
		}
		config := &ssh.ClientConfig{
			User: "root",
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(signer),
			},
			HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
			HostKeyAlgorithms: []string{"ssh-rsa"},
			Timeout:           time.Second * 5,
		}
		client, err := ssh.Dial("tcp", ip+":22", config)
		if err != nil {
			return doErr(err)
		}
		SetupSSHStatus = "Checking if device is a Vector..."
		output, err := runCmd(client, "uname -a")
		if err != nil {
			return doErr(err)
		}
		if !strings.Contains(output, "Vector") {
			return doErr(fmt.Errorf("the remote device is not a vector"))
		}
		SetupSSHStatus = "Running initial commands before transfers (screen will go blank, this is normal)..."
		_, err = runCmd(client, "mount -o rw,remount / && mount -o rw,remount,exec /data && systemctl stop anki-robot.target && mv /anki/data/assets/cozmo_resources/config/server_config.json /anki/data/assets/cozmo_resources/config/server_config.json.bak")
		if err != nil {
			if !strings.Contains(err.Error(), "Process exited with status 1") {
				return doErr(err)
			}
		}
		SetupSSHStatus = "Transferring bot setup script and certs..."
		scpClient, err := scp.NewClientBySSH(client)
		if err != nil {
			return doErr(err)
		}
		script, err := os.Open(SetupScriptPath)
		if err != nil {
			return doErr(err)
		}
		err = scpClient.CopyFile(context.Background(), script, "/data/pod-bot-install.sh", "0755")
		if err != nil {
			return doErr(err)
		}
		scpClient.Session.Close()
		serverConfig, err := os.Open("../certs/server_config.json")
		if err != nil {
			return doErr(err)
		}
		scpClient, err = scp.NewClientBySSH(client)
		if err != nil {
			return doErr(err)
		}
		err = scpClient.CopyFile(context.Background(), serverConfig, "/anki/data/assets/cozmo_resources/config/server_config.json", "0755")
		if err != nil {
			return doErr(err)
		}
		scpClient.Session.Close()
		cloud, err := os.Open("../vector-cloud/build/vic-cloud")
		if err != nil {
			return doErr(err)
		}
		SetupSSHStatus = "Transferring new vic-cloud..."
		scpClient, err = scp.NewClientBySSH(client)
		if err != nil {
			return doErr(err)
		}
		err = scpClient.CopyFile(context.Background(), cloud, "/anki/bin/vic-cloud", "0755")
		if err != nil {
			return doErr(err)
		}
		scpClient.Session.Close()
		certPath := "../certs/cert.crt"
		if vars.APIConfig.Server.EPConfig {
			certPath = "./epod/ep.crt"
		}
		cert, err := os.Open(certPath)
		if err != nil {
			return doErr(err)
		}
		scpClient, err = scp.NewClientBySSH(client)
		if err != nil {
			return doErr(err)
		}
		err = scpClient.CopyFile(context.Background(), cert, "/anki/etc/wirepod-cert.crt", "0755")
		if err != nil {
			return doErr(err)
		}
		scpClient.Session.Close()
		_, err = runCmd(client, "cp /anki/etc/wirepod-cert.crt /data/data/wirepod-cert.crt")
		if err != nil {
			return doErr(err)
		}
		SetupSSHStatus = "Generating new robot certificate (this may take a while)..."
		_, err = runCmd(client, "chmod +rwx /anki/data/assets/cozmo_resources/config/server_config.json /anki/bin/vic-cloud /data/data/wirepod-cert.crt /anki/etc/wirepod-cert.crt /data/pod-bot-install.sh && /data/pod-bot-install.sh")
		if err != nil {
			return doErr(err)
		}
		client.Close()
		SetupSSHStatus = "done"
	} else {
		return fmt.Errorf("a bot is already being setup")
	}
	return nil
}

func SSHSetup(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api-ssh/setup":
		ip := r.FormValue("ip")
		if ip == "" {
			fmt.Fprint(w, "error: must provide ip")
			return
		}
		key, _, err := r.FormFile("key")
		if err != nil {
			fmt.Fprint(w, "error: must provide ssh key ("+err.Error()+")")
			return
		}
		keyBytes, _ := io.ReadAll(key)
		if len(keyBytes) < 5 {
			fmt.Fprint(w, "error: must provide ssh key ("+err.Error()+")")
			return
		}
		go SetupBotViaSSH(ip, keyBytes)
		fmt.Fprint(w, "running")
		return
	case r.URL.Path == "/api-ssh/get_setup_status":
		fmt.Fprint(w, SetupSSHStatus)
		if SetupSSHStatus == "done" || strings.Contains(SetupSSHStatus, "error") {
			SetupSSHStatus = "not running"
		}
		return
	}
}

func RegisterSSHAPI() {
	http.HandleFunc("/api-ssh/", SSHSetup)
}
