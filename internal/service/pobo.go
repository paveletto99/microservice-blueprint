/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"time"

	"github.com/paveletto99/go-pobo/pkg/api"
	"github.com/sirupsen/logrus"
)

// Compile check *Pobo implements Runner interface
var _ api.Runner = &Pobo{}

type Pobo struct {
	// Fields
}

func NewPobo() *Pobo {
	return &Pobo{}
}

var (
	runtimePobo bool = true
)

func (n *Pobo) Run() error {
	client := api.Client{}
	server := api.Server{}
	logrus.Infof("Client: %x", client)
	logrus.Infof("Server: %x", server)
	for runtimePobo == true {
		time.Sleep(1 * time.Second)
		logrus.Infof("Sleeping...\n")
	}
	return nil
}
