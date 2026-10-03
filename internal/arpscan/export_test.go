package arpscan

import (
	"github.com/mdlayher/arp"
)

type ClientConn = clientConn

func NewClientConn(client *arp.Client) ClientConn {
	return clientConn{client: client}
}
