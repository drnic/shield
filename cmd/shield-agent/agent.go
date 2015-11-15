package main

import (
	"time"
	"encoding/json"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
)

type Agent struct {
	config *ssh.ServerConfig
}

func (agent *Agent) Serve(l net.Listener) {
	for {
		c, err := l.Accept()
		if err != nil {
			fmt.Printf("failed to accept: %s\n", err)
			return
		}

		conn, chans, reqs, err := ssh.NewServerConn(c, agent.config)
		if err != nil {
			fmt.Printf("handshake failed: %s\n", err)
			continue
		}

		go agent.handleConn(conn, chans, reqs)
	}
}

func (agent *Agent) handleConn(conn *ssh.ServerConn, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
	defer conn.Close()

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			fmt.Printf("rejecting unknown channel type: %s\n", newChannel.ChannelType())
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			fmt.Printf("failed to accept channel: %s\n", err)
			return
		}

		defer channel.Close()

		for req := range requests {
			fmt.Printf("channel request: %s\n", req.Type)

			if req.Type != "exec" {
				fmt.Printf("rejecting\n")
				req.Reply(false, nil)
				continue
			}

			request, err := ParseAgentRequest(req)
			if err != nil {
				fmt.Printf("%s\n", err)
				req.Reply(false, nil)
				continue
			}

			fmt.Printf("got an agent-request [%s]\n", request.JSON)
			req.Reply(true, nil)
			fmt.Fprintf(channel, "acceptable.\n")
			time.Sleep(5)
			fmt.Fprintf(channel, "done.\n")

			conn.Close()
		}
	}
}

type AgentRequest struct {
	JSON           string
	Operation      string `json:"operation"`
	TargetPlugin   string `json:"target_plugin"`
	TargetEndpoint string `json:"target_endpoint"`
	StorePlugin    string `json:"store_plugin"`
	StoreEndpoint  string `json:"store_endpoint"`
	RestoreKey     string `json:"restore_key"`
}

func ParseAgentRequest(req *ssh.Request) (*AgentRequest, error) {
	var raw struct {
		Value []byte
	}
	err := ssh.Unmarshal(req.Payload, &raw)
	if err != nil {
		return nil, err
	}

	request := &AgentRequest{JSON: string(raw.Value)}
	err = json.Unmarshal(raw.Value, &request)
	if err != nil {
		return nil, fmt.Errorf("malformed agent-request %v: %s\n", req.Payload, err)
	}

	if request.Operation == "" {
		return nil, fmt.Errorf("missing required 'operation' value in payload")
	}
	if request.Operation != "backup" && request.Operation != "restore" {
		return nil, fmt.Errorf("unsupported operation: '%s'", request.Operation)
	}
	if request.TargetPlugin == "" {
		return nil, fmt.Errorf("missing required 'target_plugin' value in payload")
	}
	if request.TargetEndpoint == "" {
		return nil, fmt.Errorf("missing required 'target_endpoint' value in payload")
	}
	if request.StorePlugin == "" {
		return nil, fmt.Errorf("missing required 'store_plugin' value in payload")
	}
	if request.StoreEndpoint == "" {
		return nil, fmt.Errorf("missing required 'store_endpoint' value in payload")
	}
	if request.Operation == "restore" && request.RestoreKey == "" {
		return nil, fmt.Errorf("missing required 'restore_key' value in payload (for restore operation)")
	}
	return request, nil
}
