package backend

import (
	"encoding/json"
	"log"
	"os"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func HandleLauncher(pc *proto.Conn) {
	defer pc.Close()

	for {
		msg, err := pc.Receive()
		if err != nil {
			log.Printf("connection error: %v", err)
			return
		}

		switch msg.Type {
		case proto.MsgOutput:
			var p proto.OutputPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				log.Printf("bad output payload: %v", err)
				continue
			}
			os.Stdout.Write(p.Data)

		case proto.MsgIdle:
			log.Printf("[idle] client waiting for input")
			pc.Send(proto.NewMessage(proto.MsgType, proto.TypePayload{
				Text: "list the directory contents",
			}))

		case proto.MsgExited:
			var p proto.ExitedPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				log.Printf("bad exited payload: %v", err)
				return
			}
			log.Printf("client exited with code %d signal %s", p.Code, p.Signal)
			return

		case proto.MsgStarted:
			var p proto.StartedPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				log.Printf("bad started payload: %v", err)
				continue
			}
			log.Printf("client started: %s (pid %d)", p.Client, p.Pid)

		default:
			log.Printf("unknown message type: %s", msg.Type)
		}
	}
}
