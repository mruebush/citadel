package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/citadel/citadel"
	"github.com/citadel/citadel/utils"
	"github.com/codegangsta/cli"
	"github.com/samalba/dockerclient"
)

var hostCommand = cli.Command{
	Name:   "host",
	Usage:  "run the host and connect it to the cluster",
	Action: hostAction,
	Flags: []cli.Flag{
		cli.StringFlag{"addr", "", "external ip address for the host"},
		cli.StringFlag{"docker", "unix:///var/run/docker.sock", "docker remote ip address"},
		cli.StringFlag{"listen", ":8787", "listen address"},
		cli.StringFlag{"ssl-cert", "", "SSL certificate"},
		cli.StringFlag{"ssl-key", "", "SSL key"},
		cli.StringSliceFlag{"labels", &cli.StringSlice{}, "labels to apply as attributes of the host"},
	},
}

func hostAction(context *cli.Context) {
	validateContext(context)

	host, err := citadel.NewHost(getHostId(), context.StringSlice("labels"), getClient(context), logger)
	if err != nil {
		logger.WithField("error", err).Fatal("create host")
	}

	server := citadel.NewServer(host)
	go waitForInterrupt(server)

	if err := http.ListenAndServeTLS(context.String("addr"), context.String("ssl-cert"), context.String("ssl-key"), server); err != nil {
		logger.WithField("error", err).Fatal("listen and serve")
	}
}

func waitForInterrupt(s *citadel.Server) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	for _ = range sigChan {
		if err := s.Close(); err != nil {
			logger.WithField("error", err).Fatal("closing server")
		}
		os.Exit(0)
	}
}

func getHostId() string {
	id, err := utils.GetMachineID()
	if err != nil {
		logger.WithField("error", err).Fatal("unable to read machine id")
	}
	return id
}

func getClient(context *cli.Context) *dockerclient.DockerClient {
	client, err := dockerclient.NewDockerClient(context.String("docker"))
	if err != nil {
		logger.WithField("error", err).Fatal("unable to connect to docker")
	}

	return client
}

func validateContext(context *cli.Context) {
	switch {
	case context.String("addr") == "":
		logger.Fatal("addr must have a value")
	}
}
