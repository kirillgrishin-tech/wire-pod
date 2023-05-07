package main

import (
	"github.com/kirillgrishin-tech/chipper/pkg/initwirepod"
	stt "github.com/kirillgrishin-tech/chipper/pkg/wirepod/stt/leopard"
)

func main() {
	initwirepod.StartFromProgramInit(stt.Init, stt.STT, stt.Name)
}
