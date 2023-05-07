package main

import (
	"github.com/kirillgrishin-tech/chipper/pkg/initwirepod"
	stt "github.com/kirillgrishin-tech/chipper/pkg/wirepod/stt/vosk"
)

func main() {
	initwirepod.StartFromProgramInit(stt.Init, stt.STT, stt.Name)
}
