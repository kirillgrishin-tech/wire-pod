package processreqs

import (
	"fmt"
	"io"

	sr "github.com/kirillgrishin-tech/chipper/pkg/wirepod/speechrequest"
	"github.com/soundhound/houndify-sdk-go"
)

func StreamAudioToHoundify(sreq sr.SpeechRequest, client houndify.Client) string {
	var err error
	rp, wp := io.Pipe()
	req := houndify.VoiceRequest{
		AudioStream: rp,
		UserID:      sreq.Device,
		RequestID:   sreq.Session,
	}
	done := make(chan bool)
	speechDone := false
	go func(wp *io.PipeWriter) {
		defer wp.Close()

		for {
			select {
			case <-done:
				return
			default:
				var chunk []byte
				sreq, chunk, err = sr.GetNextStreamChunkOpus(sreq)
				sreq, speechDone = sr.DetectEndOfSpeech(sreq)
				if err != nil {
					fmt.Println("End of stream")
					return
				}
				wp.Write(chunk)
				if speechDone {
					return
				}
			}
		}
	}(wp)

	partialTranscripts := make(chan houndify.PartialTranscript)
	go func() {
		for partial := range partialTranscripts {
			if *partial.SafeToStopAudio {
				fmt.Println("SafeToStopAudio recieved")
				done <- true
				return
			}
		}
	}()

	serverResponse, err := client.VoiceSearch(req, partialTranscripts)
	if err != nil {
		fmt.Println(err)
		fmt.Println(serverResponse)
	}
	return serverResponse
}
