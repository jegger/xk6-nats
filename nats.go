package nats

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/grafana/sobek"
	natsio "github.com/nats-io/nats.go"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
)

func init() {
	modules.Register("k6/x/nats", new(RootModule))
}

// RootModule is the global module object type. It is instantiated once per test
// run and will be used to create k6/x/nats module instances for each VU.
type RootModule struct{}

// ModuleInstance represents an instance of the module for every VU.
type Nats struct {
	conn    *natsio.Conn
	vu      modules.VU
	exports map[string]interface{}
}

// Ensure the interfaces are implemented correctly.
var (
	_ modules.Instance = &Nats{}
	_ modules.Module   = &RootModule{}
)

// NewModuleInstance implements the modules.Module interface and returns
// a new instance for each VU.
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance {
	mi := &Nats{
		vu:      vu,
		exports: make(map[string]interface{}),
	}

	mi.exports["Nats"] = mi.client

	return mi
}

// Exports implements the modules.Instance interface and returns the exports
// of the JS module.
func (mi *Nats) Exports() modules.Exports {
	return modules.Exports{
		Named: mi.exports,
	}
}

func (n *Nats) client(c sobek.ConstructorCall) *sobek.Object {
	rt := n.vu.Runtime()

	var cfg Configuration
	err := rt.ExportTo(c.Argument(0), &cfg)
	if err != nil {
		common.Throw(rt, fmt.Errorf("Nats constructor expect Configuration as it's argument: %w", err))
	}

	natsOptions := natsio.GetDefaultOptions()
	natsOptions.Servers = cfg.Servers
	if cfg.Unsafe {
		natsOptions.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}
	if cfg.Token != "" {
		natsOptions.Token = cfg.Token
	}

	if headers := cfg.wsHeaders(); headers != nil {
		natsOptions.WebSocketConnectionHeaders = headers
	}
	if cfg.InboxPrefix != "" {
		natsOptions.InboxPrefix = cfg.InboxPrefix
	}

	conn, err := natsOptions.Connect()
	if err != nil {
		if (cfg.JWT != "" || cfg.CookieJWT != "") && err.Error() == "EOF" {
			common.Throw(rt, fmt.Errorf("connection failed (JWT may be invalid or expired): %w", err))
		}
		common.Throw(rt, err)
	}

	return rt.ToValue(&Nats{
		vu:   n.vu,
		conn: conn,
	}).ToObject(rt)
}

func (n *Nats) Close() {
	if n.conn != nil {
		n.conn.Close()
	}
}

func (n *Nats) Publish(topic, message string, headers ...map[string]string) error {
	if n.conn == nil {
		return fmt.Errorf("the connection is not valid")
	}

	if len(headers) > 0 && headers[0] != nil {
		msg := &natsio.Msg{
			Subject: topic,
			Data:    []byte(message),
			Header:  natsio.Header{},
		}
		for k, v := range headers[0] {
			msg.Header.Set(k, v)
		}
		return n.conn.PublishMsg(msg)
	}

	return n.conn.Publish(topic, []byte(message))
}

func (n *Nats) Subscribe(topic string, handler MessageHandler) error {
	if n.conn == nil {
		return fmt.Errorf("the connection is not valid")
	}

	_, err := n.conn.Subscribe(topic, func(msg *natsio.Msg) {
		message := Message{
			Data:  string(msg.Data),
			Topic: msg.Subject,
		}
		handler(message)
	})

	return err
}

func (n *Nats) Request(subject, data string, headers ...map[string]string) (Message, error) {
	if n.conn == nil {
		return Message{}, fmt.Errorf("the connection is not valid")
	}

	if len(headers) > 0 && headers[0] != nil {
		msg := &natsio.Msg{
			Subject: subject,
			Data:    []byte(data),
			Header:  natsio.Header{},
		}
		for k, v := range headers[0] {
			msg.Header.Set(k, v)
		}
		resp, err := n.conn.RequestMsg(msg, 5*time.Second)
		if err != nil {
			return Message{}, err
		}
		return Message{
			Data:  string(resp.Data),
			Topic: resp.Subject,
		}, nil
	}

	msg, err := n.conn.Request(subject, []byte(data), 5*time.Second)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Data:  string(msg.Data),
		Topic: msg.Subject,
	}, nil
}

type Configuration struct {
	Servers   []string `js:"servers"`
	Unsafe    bool     `js:"unsafe"`
	Token     string   `js:"token"`
	JWT          string `js:"jwt"`
	CookieJWT    string `js:"cookieJwt"`
	InboxPrefix  string `js:"inboxPrefix"`
}

func (c *Configuration) wsHeaders() http.Header {
	if c.JWT == "" && c.CookieJWT == "" {
		return nil
	}
	h := make(http.Header)
	if c.JWT != "" {
		h.Set("Authorization", "Bearer "+c.JWT)
	}
	if c.CookieJWT != "" {
		h.Set("Cookie", "nats="+c.CookieJWT)
	}
	return h
}

type Message struct {
	Data  string
	Topic string
}

type MessageHandler func(Message)
