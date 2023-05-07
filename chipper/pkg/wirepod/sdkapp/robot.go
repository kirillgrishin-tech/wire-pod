package sdkapp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kirillgrishin-tech/chipper/pkg/logger"
	"github.com/kirillgrishin-tech/chipper/pkg/vars"
	"github.com/kirillgrishin-tech/vector-go-sdk/pkg/vector"
	"github.com/kirillgrishin-tech/vector-go-sdk/pkg/vectorpb"
)

var robots []Robot
var timerStopIndexes []int
var inhibitCreation bool

type Robot struct {
	ESN               string
	GUID              string
	Target            string
	Vector            *vector.Vector
	BcAssumption      bool
	CamStreaming      bool
	EventStreamClient vectorpb.ExternalInterface_EventStreamClient
	EventsStreaming   bool
	StimState         float32
	ConnTimer         int32
	Ctx               context.Context
}

func newRobot(serial string) (Robot, int, error) {
	inhibitCreation = true
	var RobotObj Robot

	// generate context
	RobotObj.Ctx = context.Background()

	// find robot info in BotInfo
	matched := false
	for _, robot := range vars.BotInfo.Robots {
		if strings.EqualFold(serial, robot.Esn) {
			RobotObj.ESN = strings.TrimSpace(strings.ToLower(serial))
			RobotObj.Target = robot.IPAddress + ":443"
			matched = true
			if robot.GUID == "" {
				robot.GUID = vars.BotInfo.GlobalGUID
				RobotObj.GUID = vars.BotInfo.GlobalGUID
			} else {
				RobotObj.GUID = robot.GUID
			}
			logger.Println("Connecting to " + serial + " with GUID " + RobotObj.GUID)
		}
	}
	if !matched {
		inhibitCreation = false
		return RobotObj, 0, fmt.Errorf("error: robot not found in SDK info file")
	}

	// create Vector instance
	var err error
	RobotObj.Vector, err = vector.New(
		vector.WithTarget(RobotObj.Target),
		vector.WithSerialNo(RobotObj.ESN),
		vector.WithToken(RobotObj.GUID),
	)
	if err != nil {
		inhibitCreation = false
		return RobotObj, 0, err
	}

	// connection check
	_, err = RobotObj.Vector.Conn.BatteryState(context.Background(), &vectorpb.BatteryStateRequest{})
	if err != nil {
		inhibitCreation = false
		return RobotObj, 0, err
	}

	// create client for event stream
	RobotObj.EventStreamClient, err = RobotObj.Vector.Conn.EventStream(
		RobotObj.Ctx,
		&vectorpb.EventRequest{
			ListType: &vectorpb.EventRequest_WhiteList{
				WhiteList: &vectorpb.FilterList{
					// this will be used only for stimulation graph for now
					List: []string{"stimulation_info"},
				},
			},
		},
	)
	if err != nil {
		inhibitCreation = false
		return RobotObj, 0, err
	}
	RobotObj.CamStreaming = false
	RobotObj.EventsStreaming = false

	// we have confirmed robot connection works, append to list of bots
	robots = append(robots, RobotObj)
	robotIndex := len(robots) - 1

	// begin inactivity timer
	go connTimer(robotIndex)

	inhibitCreation = false
	return RobotObj, robotIndex, nil
}

func getRobot(serial string) (Robot, int, error) {
	// look in robot list
	for {
		if !inhibitCreation {
			break
		}
		time.Sleep(time.Second / 2)
	}
	for index, robot := range robots {
		if strings.EqualFold(serial, robot.ESN) {
			return robot, index, nil
		}
	}
	return newRobot(serial)
}

// if connection is inactive for more than 5 minutes, remove robot
// run this as a goroutine
func connTimer(ind int) {
	robots[ind].ConnTimer = 0
	for {
		time.Sleep(time.Second)
		// check if timer needs to be stopped
		for _, num := range timerStopIndexes {
			if num == ind {
				logger.Println("Conn timer for robot index " + strconv.Itoa(ind) + " stopping")
				var newIndexes []int
				for _, num := range timerStopIndexes {
					if num != ind {
						newIndexes = append(newIndexes, num)
					}
				}
				timerStopIndexes = newIndexes
				return
			}
		}
		if robots[ind].ConnTimer >= 300 {
			logger.Println("Closing SDK connection for " + robots[ind].ESN + ", source: connTimer")
			removeRobot(robots[ind].ESN, "connTimer")
			return
		}
		robots[ind].ConnTimer = robots[ind].ConnTimer + 1
	}
}

func removeRobot(serial, source string) {
	inhibitCreation = true
	var newRobots []Robot
	for ind, robot := range robots {
		if !strings.EqualFold(serial, robot.ESN) {
			newRobots = append(newRobots, robot)
		} else {
			if source == "server" {
				timerStopIndexes = append(timerStopIndexes, ind)
			}
			if robots[ind].CamStreaming {
				robots[ind].CamStreaming = false
				// give it time to stop the camera stream
				time.Sleep(time.Second * 2)
			}
			if robots[ind].EventsStreaming {
				robots[ind].EventsStreaming = false
				time.Sleep(time.Second * 2)
			}
		}
	}
	robots = newRobots
	inhibitCreation = false
}
