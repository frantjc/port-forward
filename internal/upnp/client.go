package upnp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	xslice "github.com/frantjc/x/slice"
	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/huin/goupnp/dcps/internetgateway2"
)

// Protocol is the protocol of a port.
type Protocol string

var (
	// ProtocolUDP is the UDP Protocol.
	ProtocolUDP Protocol = "UDP"
	// ProtocolTCP is the TCP Protocol.
	ProtocolTCP Protocol = "TCP"
)

// PortMapping is the mapping of an external port
// to an internal address.
type PortMapping struct {
	RemoteHost     string
	ExternalPort   int32
	Protocol       Protocol
	InternalPort   int32
	InternalClient net.IP
	Enabled        bool
	Description    string
	LeaseDuration  time.Duration
}

type Client struct {
	GoUPnPClient GoUPnPClient
}

func (c *Client) GetExternalIPAddress(ctx context.Context) (net.IP, error) {
	ips, err := c.GoUPnPClient.GetExternalIPAddressCtx(ctx)
	if err != nil {
		return nil, err
	}

	ip := net.ParseIP(ips)
	if ip == nil {
		return nil, fmt.Errorf("unable to parse ip: %s", ips)
	}

	return ip, nil
}

func (c *Client) AddPortMapping(ctx context.Context, pm *PortMapping) error {
	return c.GoUPnPClient.AddPortMappingCtx(ctx,
		pm.RemoteHost,
		uint16(pm.ExternalPort),
		string(pm.Protocol),
		uint16(pm.InternalPort),
		pm.InternalClient.To4().String(),
		pm.Enabled,
		pm.Description,
		uint32(pm.LeaseDuration.Seconds()),
	)
}

func (c *Client) GetServiceIPAddress(context.Context) (net.IP, error) {
	location := c.GoUPnPClient.GetServiceClient().Location

	ips, err := net.LookupIP(location.Hostname())
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		if ip != nil {
			return ip, nil
		}
	}

	return nil, fmt.Errorf(`no IP addresses found for UPnP service location "%s"`, location)
}


func (c *Client) GetSourceIPAddress(context.Context) net.IP {
	return c.GoUPnPClient.GetServiceClient().LocalAddr()
}

type GoUPnPClient interface {
	GetExternalIPAddressCtx(context.Context) (string, error)
	GetServiceClient() *goupnp.ServiceClient
	AddPortMappingCtx(
		context.Context,
		string,
		uint16,
		string,
		uint16,
		string,
		bool,
		string,
		uint32,
	) error
}

var (
	ErrNoClients = errors.New("no clients found")
)

type getClients func(context.Context) ([]GoUPnPClient, []error, error)

type NewClientOpts struct {
	getClients []getClients
}

type NewClientOpt func(*NewClientOpts)

func castToGoUPnPClients[client GoUPnPClient](clients []client) []GoUPnPClient {
	return xslice.Map(clients, func(cli client, _ int) GoUPnPClient {
		return cli
	})
}

func WithIG2WANIPConnection2(opts *NewClientOpts) {
	if opts.getClients == nil {
		opts.getClients = []getClients{}
	}

	opts.getClients = append(opts.getClients, func(ctx context.Context) ([]GoUPnPClient, []error, error) {
		clients, errs, err := internetgateway2.NewWANIPConnection2ClientsCtx(ctx)
		return castToGoUPnPClients(clients), errs, err
	})
}

func WithIG2WANIPConnection1(opts *NewClientOpts) {
	if opts.getClients == nil {
		opts.getClients = []getClients{}
	}

	opts.getClients = append(opts.getClients, func(ctx context.Context) ([]GoUPnPClient, []error, error) {
		clients, errs, err := internetgateway2.NewWANIPConnection1ClientsCtx(ctx)
		return castToGoUPnPClients(clients), errs, err
	})
}

func WithIG2WANPPPConnection1(opts *NewClientOpts) {
	if opts.getClients == nil {
		opts.getClients = []getClients{}
	}

	opts.getClients = append(opts.getClients, func(ctx context.Context) ([]GoUPnPClient, []error, error) {
		clients, errs, err := internetgateway2.NewWANPPPConnection1ClientsCtx(ctx)
		return castToGoUPnPClients(clients), errs, err
	})
}

func WithIG1WANIP1Connection1(opts *NewClientOpts) {
	if opts.getClients == nil {
		opts.getClients = []getClients{}
	}

	opts.getClients = append(opts.getClients, func(ctx context.Context) ([]GoUPnPClient, []error, error) {
		clients, errs, err := internetgateway1.NewWANIPConnection1ClientsCtx(ctx)
		return castToGoUPnPClients(clients), errs, err
	})
}

func WithIG1WANPPP1Connection1(opts *NewClientOpts) {
	if opts.getClients == nil {
		opts.getClients = []getClients{}
	}

	opts.getClients = append(opts.getClients, func(ctx context.Context) ([]GoUPnPClient, []error, error) {
		clients, errs, err := internetgateway1.NewWANPPPConnection1ClientsCtx(ctx)
		return castToGoUPnPClients(clients), errs, err
	})
}

func WithAnyConnection(opts *NewClientOpts) {
	opts.getClients = []getClients{}
	WithIG2WANIPConnection2(opts)
	WithIG2WANIPConnection1(opts)
	WithIG2WANPPPConnection1(opts)
	WithIG1WANIP1Connection1(opts)
	WithIG1WANPPP1Connection1(opts)
}

func WithGoUPnPClient(client GoUPnPClient) NewClientOpt {
	return func(opts *NewClientOpts) {
		opts.getClients = []getClients{
			func(ctx context.Context) ([]GoUPnPClient, []error, error) {
				return []GoUPnPClient{client}, nil, nil
			},
		}
	}
}

func NewClient(ctx context.Context, opts ...NewClientOpt) (*Client, error) {
	o := &NewClientOpts{}

	for _, opt := range opts {
		opt(o)
	}

	for _, getClient := range o.getClients {
		GoUPnPClient, err := getOneGoUPnPClient(ctx, getClient)
		if err != nil {
			if errors.Is(err, ErrNoClients) {
				continue
			}

			return nil, err
		}

		return &Client{GoUPnPClient}, nil
	}

	return nil, ErrNoClients
}

func getOneGoUPnPClient(ctx context.Context, f getClients) (GoUPnPClient, error) {
	clients, _, err := f(ctx)
	if err != nil {
		return nil, err
	} else if len(clients) == 0 {
		return nil, ErrNoClients
	}

	return clients[0], nil
}
