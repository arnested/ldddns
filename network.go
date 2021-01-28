package main

import (
	"context"

	"github.com/docker/docker/client"
	"github.com/godbus/dbus/v5"
)

const networkChannelSize = 20

func networkState(ctx context.Context, docker *client.Client, egs *EntryGroups, conn *dbus.Conn) {
	c := make(chan *dbus.Signal, networkChannelSize)
	conn.Signal(c)

	for v := range c {
		if v.Name == "org.freedesktop.NetworkManager.StateChanged" && v.Body[0].(uint32) == 0x32 {
			logf(PriInfo, "NetworkManager state connected local")
			handleExistingContainers(ctx, docker, egs)
		}
	}
}
