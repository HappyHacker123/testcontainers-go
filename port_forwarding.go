package testcontainers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/testcontainers/testcontainers-go/internal/core/network"
)

const (
	// hubSshdImage {
	sshdImage string = "testcontainers/sshd:1.2.0"
	// }

	// HostInternal is the internal hostname used to reach the host from the container,
	// using the SSHD container as a bridge.
	HostInternal string = "host.testcontainers.internal"
	user         string = "root"
	sshPort             = "22"
)

// sshPassword is a random password generated for the SSHD container.
var sshPassword = uuid.NewString()

// exposeHostPorts performs all the necessary steps to expose the host ports to the container, leveraging
// the SSHD container to create the tunnel, and the container lifecycle hooks to manage the tunnel lifecycle.
// At least one port must be provided to expose.
// The steps are:
// 1. Create a new SSHD container.
// 2. Expose the host ports to the container after the container is ready.
// 3. Close the SSH sessions before killing the container.
func exposeHostPorts(ctx context.Context, req *Request, p ...int) (ContainerLifecycleHooks, error) {
	var sshdConnectHook ContainerLifecycleHooks

	if len(p) == 0 {
		return sshdConnectHook, fmt.Errorf("no ports to expose")
	}

	// Use the first network of the container to connect to the SSHD container.
	var sshdFirstNetwork string
	if len(req.Networks) > 0 {
		sshdFirstNetwork = req.Networks[0]
	}

	if sshdFirstNetwork == "bridge" && len(req.Networks) > 1 {
		sshdFirstNetwork = req.Networks[1]
	}

	opts := []RequestCustomizer{}
	if len(req.Networks) > 0 {
		// get the first network of the container to connect the SSHD container to it.
		nw, err := network.GetByName(ctx, sshdFirstNetwork)
		if err != nil {
			return sshdConnectHook, fmt.Errorf("failed to get the network: %w", err)
		}

		// WithNetwork reuses an already existing network, attaching the container to it.
		// Finally it sets the network alias on that network to the given alias.
		// TODO: Using an anonymous function to avoid cyclic dependencies with the network package.
		withNetwork := func(aliases []string, networkName string) CustomizeRequestOption {
			return func(req *Request) error {
				// attaching to the network because it was created with success or it already existed.
				req.Networks = append(req.Networks, networkName)

				if req.NetworkAliases == nil {
					req.NetworkAliases = make(map[string][]string)
				}
				req.NetworkAliases[networkName] = aliases
				return nil
			}
		}

		opts = append(opts, withNetwork([]string{HostInternal}, nw.Name))
	}

	// start the SSHD container with the provided options
	sshdContainer, err := newSshdContainer(ctx, opts...)
	if err != nil {
		return sshdConnectHook, fmt.Errorf("failed to create the SSH server: %w", err)
	}

	// IP in the first network of the container
	sshdIP, err := sshdContainer.ContainerIP(context.Background())
	if err != nil {
		return sshdConnectHook, fmt.Errorf("failed to get IP for the SSHD container: %w", err)
	}

	if req.HostConfigModifier == nil {
		req.HostConfigModifier = func(hostConfig *container.HostConfig) {}
	}

	// do not override the original HostConfigModifier
	originalHCM := req.HostConfigModifier
	req.HostConfigModifier = func(hostConfig *container.HostConfig) {
		// adding the host internal alias to the container as an extra host
		// to allow the container to reach the SSHD container.
		hostConfig.ExtraHosts = append(hostConfig.ExtraHosts, fmt.Sprintf("%s:%s", HostInternal, sshdIP))

		modes := []container.NetworkMode{container.NetworkMode(sshdFirstNetwork), "none", "host"}
		// if the container is not in one of the modes, attach it to the first network of the SSHD container
		found := false
		for _, mode := range modes {
			if hostConfig.NetworkMode == mode {
				found = true
				break
			}
		}
		if !found {
			req.Networks = append(req.Networks, sshdFirstNetwork)
		}

		// invoke the original HostConfigModifier with the updated hostConfig
		originalHCM(hostConfig)
	}

	// after the container is ready, create the SSH tunnel
	// for each exposed port from the host.
	sshdConnectHook = ContainerLifecycleHooks{
		PostReadies: []StartedContainerHook{
			func(ctx context.Context, c StartedContainer) error {
				return sshdContainer.exposeHostPort(ctx, req.HostAccessPorts...)
			},
		},
		PreTerminates: []StartedContainerHook{
			func(ctx context.Context, _ StartedContainer) error {
				// before killing the container, close the SSH sessions
				return sshdContainer.Terminate(ctx)
			},
		},
	}

	return sshdConnectHook, nil
}

// newSshdContainer creates a new SSHD container with the provided options.
func newSshdContainer(ctx context.Context, opts ...RequestCustomizer) (*sshdContainer, error) {
	req := Request{
		Image:           sshdImage,
		HostAccessPorts: []int{}, // empty list because it does not need any port
		ExposedPorts:    []string{sshPort},
		Env:             map[string]string{"PASSWORD": sshPassword},
		Started:         true,
	}

	for _, opt := range opts {
		if err := opt.Customize(&req); err != nil {
			return nil, err
		}
	}

	dc, err := New(ctx, req)
	if err != nil {
		return nil, err
	}

	sshd := &sshdContainer{
		DockerContainer: dc,
		portForwarders:  []PortForwarder{},
	}

	sshClientConfig, err := configureSSHConfig(ctx, sshd)
	if err != nil {
		// return the container and the error to the caller to handle it
		return sshd, err
	}

	sshd.sshConfig = sshClientConfig

	return sshd, nil
}

// sshdContainer represents the SSHD container type used for the port forwarding container.
// It's an internal type that extends the DockerContainer type, to add the SSH tunneling capabilities.
type sshdContainer struct {
	*DockerContainer
	port           string
	sshConfig      *ssh.ClientConfig
	portForwarders []PortForwarder
}

// Terminate stops the container and closes the SSH session
func (sshdC *sshdContainer) Terminate(ctx context.Context) error {
	for _, pfw := range sshdC.portForwarders {
		pfw.Close(ctx)
	}

	return sshdC.DockerContainer.Terminate(ctx)
}

func configureSSHConfig(ctx context.Context, sshdC *sshdContainer) (*ssh.ClientConfig, error) {
	mappedPort, err := sshdC.MappedPort(ctx, sshPort)
	if err != nil {
		return nil, err
	}
	sshdC.port = mappedPort.Port()

	sshConfig := ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.Password(sshPassword)},
		Timeout:         30 * time.Second,
	}

	return &sshConfig, nil
}

func (sshdC *sshdContainer) exposeHostPort(ctx context.Context, ports ...int) error {
	for _, port := range ports {
		pw := NewPortForwarder(fmt.Sprintf("localhost:%s", sshdC.port), sshdC.sshConfig, port, port)
		sshdC.portForwarders = append(sshdC.portForwarders, *pw)

		go pw.Forward(ctx) //nolint:errcheck // Nothing we can usefully do with the error
	}

	var err error

	// continue when all port forwarders have created the connection
	for _, pfw := range sshdC.portForwarders {
		err = errors.Join(err, <-pfw.connectionCreated)
	}

	return err
}

type PortForwarder struct {
	sshDAddr          string
	sshConfig         *ssh.ClientConfig
	remotePort        int
	localPort         int
	connectionCreated chan error    // used to signal that the connection has been created, so the caller can proceed
	terminateChan     chan struct{} // used to signal that the connection has been terminated
}

func NewPortForwarder(sshDAddr string, sshConfig *ssh.ClientConfig, remotePort, localPort int) *PortForwarder {
	return &PortForwarder{
		sshDAddr:          sshDAddr,
		sshConfig:         sshConfig,
		remotePort:        remotePort,
		localPort:         localPort,
		connectionCreated: make(chan error),
		terminateChan:     make(chan struct{}),
	}
}

func (pf *PortForwarder) Close(ctx context.Context) {
	close(pf.terminateChan)
	close(pf.connectionCreated)
}

func (pf *PortForwarder) Forward(ctx context.Context) error {
	client, err := ssh.Dial("tcp", pf.sshDAddr, pf.sshConfig)
	if err != nil {
		err = fmt.Errorf("error dialing ssh server: %w", err)
		pf.connectionCreated <- err
		return err
	}
	defer client.Close()

	listener, err := client.Listen("tcp", fmt.Sprintf("localhost:%d", pf.remotePort))
	if err != nil {
		err = fmt.Errorf("error listening on remote port: %w", err)
		pf.connectionCreated <- err
		return err
	}
	defer listener.Close()

	// signal that the connection has been created
	pf.connectionCreated <- nil

	// check if the context or the terminateChan has been closed
	select {
	case <-ctx.Done():
		if err := listener.Close(); err != nil {
			return fmt.Errorf("error closing listener: %w", err)
		}
		if err := client.Close(); err != nil {
			return fmt.Errorf("error closing client: %w", err)
		}
		return nil
	case <-pf.terminateChan:
		if err := listener.Close(); err != nil {
			return fmt.Errorf("error closing listener: %w", err)
		}
		if err := client.Close(); err != nil {
			return fmt.Errorf("error closing client: %w", err)
		}
		return nil
	default:
	}

	for {
		remote, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("error accepting connection: %w", err)
		}

		go pf.runTunnel(ctx, remote)
	}
}

// runTunnel runs a tunnel between two connections; as soon as one connection
// reaches EOF or reports an error, both connections are closed and this
// function returns.
func (pf *PortForwarder) runTunnel(ctx context.Context, remote net.Conn) {
	var dialer net.Dialer
	local, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("localhost:%d", pf.localPort))
	if err != nil {
		remote.Close()
		return
	}
	defer local.Close()

	defer remote.Close()
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(local, remote) //nolint:errcheck // Nothing we can usefully do with the error
		done <- struct{}{}
	}()

	go func() {
		io.Copy(remote, local) //nolint:errcheck // Nothing we can usefully do with the error
		done <- struct{}{}
	}()

	<-done
}
