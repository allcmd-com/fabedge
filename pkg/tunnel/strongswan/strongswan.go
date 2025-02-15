// Copyright 2021 BoCloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package strongswan

import (
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/fabedge/fabedge/pkg/tunnel"
	"github.com/strongswan/govici/vici"
)

var _ tunnel.Manager = &StrongSwanManager{}

type StrongSwanManager struct {
	socketPath  string
	certsPath   string
	startAction string
}

type connection struct {
	LocalAddrs  []string               `vici:"local_addrs"`
	RemoteAddrs []string               `vici:"remote_addrs,omitempty"`
	Proposals   []string               `vici:"proposals,omitempty"`
	Encap       string                 `vici:"encap"` //yes,no
	LocalAuth   authConf               `vici:"local"`
	RemoteAuth  authConf               `vici:"remote"`
	Children    map[string]childSAConf `vici:"children"`
	IF_ID_IN    *uint                  `vici:"if_id_in"`
	IF_ID_OUT   *uint                  `vici:"if_id_out"`
}

type authConf struct {
	ID         string   `vici:"id"`
	AuthMethod string   `vici:"auth"` // (psk|pubkey)
	Certs      []string `vici:"certs,omitempty"`
}

type childSAConf struct {
	LocalTS     []string `vici:"local_ts"`
	RemoteTS    []string `vici:"remote_ts"`
	StartAction string   `vici:"start_action"` //none,trap,start
	CloseAction string   `vici:"close_action"` //none,clear,hold,restart
}

func New(opts ...option) (*StrongSwanManager, error) {
	manager := &StrongSwanManager{
		socketPath:  "/var/run/charon.vici",
		certsPath:   filepath.Join("/etc/ipsec.d", "certs"),
		startAction: "trap",
	}

	for _, opt := range opts {
		opt(manager)
	}

	return manager, nil
}

func (m StrongSwanManager) ListConnNames() ([]string, error) {
	var names []string

	err := m.do(func(session *vici.Session) error {
		msg, err := session.CommandRequest("get-conns", vici.NewMessage())
		if err != nil {
			return err
		}

		names = msg.Get("conns").([]string)
		return nil
	})

	return names, err
}

func (m StrongSwanManager) LoadConn(cnf tunnel.ConnConfig) error {
	certs, err := m.getCerts(cnf.LocalCerts)
	if err != nil {
		return err
	}

	conn := connection{
		LocalAddrs:  cnf.LocalAddress,
		RemoteAddrs: cnf.RemoteAddress,
		Proposals:   []string{"aes128-sha256-x25519"},
		Encap:       "no",
		IF_ID_IN:    cnf.IF_ID_IN,
		IF_ID_OUT:   cnf.IF_ID_OUT,
		LocalAuth: authConf{
			ID:         cnf.LocalID,
			AuthMethod: "pubkey",
			Certs:      certs,
		},
		RemoteAuth: authConf{
			ID:         cnf.RemoteID,
			AuthMethod: "pubkey",
		},
		Children: map[string]childSAConf{
			fmt.Sprintf("%s-%d", cnf.Name, 1): {
				LocalTS:     cnf.LocalSubnets,
				RemoteTS:    cnf.RemoteSubnets,
				StartAction: m.startAction,
			},
		},
	}

	c, err := vici.MarshalMessage(conn)
	if err != nil {
		return err
	}

	msg := vici.NewMessage()
	if err := msg.Set(cnf.Name, c); err != nil {
		return err
	}

	return m.do(func(session *vici.Session) error {
		_, err := session.CommandRequest("load-conn", msg)
		return err
	})
}

func (m StrongSwanManager) UnloadConn(name string) error {
	return m.do(func(session *vici.Session) error {
		msg := vici.NewMessage()
		if err := msg.Set("name", name); err != nil {
			return err
		}

		_, err := session.CommandRequest("unload-conn", msg)
		return err
	})
}

func (m StrongSwanManager) do(fn func(session *vici.Session) error) error {
	session, err := vici.NewSession(vici.WithSocketPath(m.socketPath))
	if err != nil {
		return err
	}
	defer session.Close()

	return fn(session)
}

func (m StrongSwanManager) getCerts(filenames []string) ([]string, error) {
	var certs []string
	for _, filename := range filenames {
		cert, err := m.getCert(filename)
		if err != nil {
			return certs, err
		}

		certs = append(certs, cert)
	}

	return certs, nil
}

func (m StrongSwanManager) getCert(filename string) (string, error) {
	if !strings.HasPrefix(filename, "/") {
		filename = filepath.Join(m.certsPath, filename)
	}

	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}

	block, _ := pem.Decode(raw)
	pemBytes := pem.EncodeToMemory(block)

	return string(pemBytes), nil
}
