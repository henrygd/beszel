package server

import "golang.org/x/crypto/ssh"

type Server struct {
	Host   string
	Port   string
	Status string
	Client *ssh.Client
}

func NewServer(host, port string) *Server {
	return &Server{
		Host: host,
		Port: port,
	}
}
