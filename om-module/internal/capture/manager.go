package capture

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Parz1val02/OM_module/internal/collector"
	dockerclient "github.com/Parz1val02/OM_module/internal/docker"
)

const (
	// networkName is the Docker network shared by all core containers.
	networkName = "docker_open5gs_default"

	// generationPollInterval is how long the manager waits between
	// generation detection retries when no core containers are running yet.
	generationPollInterval = 5 * time.Second

	// restartBackoffInitial is the starting backoff after a tshark crash.
	restartBackoffInitial = 5 * time.Second

	// restartBackoffMax caps the exponential backoff.
	restartBackoffMax = 60 * time.Second
)

// Status holds a point-in-time view of the capture manager state.
// It is returned by the Status() method and exposed via the API.
type Status struct {
	Running       bool
	Interface     string
	Generation    string
	PacketsTotal  uint64
	Packets4G     uint64
	Packets5G     uint64
	RestartCount  uint64
	UptimeSeconds float64
	ActiveProcs   int
}

// Manager owns the tshark subprocess and feeds parsed packets to the correlator.
// It handles:
//   - Dynamic bridge interface discovery via the Docker socket
//   - Active generation detection via the collector snapshot
//   - Auto-restart with exponential backoff after subprocess crashes
//   - Graceful shutdown when the context is cancelled
type Manager struct {
	docker           *dockerclient.Client
	snap             *collector.Snapshot
	mcc              string
	mnc              string
	captureInterface string // "auto" or explicit interface name

	// out is the channel the correlator reads from.
	out chan Packet

	// internal state (atomic where accessed from multiple goroutines)
	iface      string
	generation string
	restarts   atomic.Uint64
	packets4g  atomic.Uint64
	packets5g  atomic.Uint64
	startTime  time.Time
	mu         sync.RWMutex // protects iface and generation
}

// NewManager creates a Manager. mcc and mnc are used to reconstruct full IMSI
// values from 5G SUCI MSIN (e.g. mcc="001", mnc="01").
// captureInterface should be "auto" for dynamic discovery or an explicit
// interface name like "br-abc123" to bypass discovery.
func NewManager(
	docker *dockerclient.Client,
	snap *collector.Snapshot,
	mcc, mnc string,
	captureInterface string,
) *Manager {
	return &Manager{
		docker:           docker,
		snap:             snap,
		mcc:              mcc,
		mnc:              mnc,
		captureInterface: captureInterface,
		out:              make(chan Packet, 512),
	}
}

// Packets returns the channel that delivers parsed packets to consumers.
// The correlator should range over this channel.
func (m *Manager) Packets() <-chan Packet {
	return m.out
}

// Status returns a snapshot of the current capture manager state.
func (m *Manager) Status() Status {
	m.mu.RLock()
	iface := m.iface
	gen := m.generation
	m.mu.RUnlock()

	uptime := 0.0
	if !m.startTime.IsZero() {
		uptime = time.Since(m.startTime).Seconds()
	}

	return Status{
		Running:       iface != "",
		Interface:     iface,
		Generation:    gen,
		PacketsTotal:  m.packets4g.Load() + m.packets5g.Load(),
		Packets4G:     m.packets4g.Load(),
		Packets5G:     m.packets5g.Load(),
		RestartCount:  m.restarts.Load(),
		UptimeSeconds: uptime,
	}
}

// Run starts the capture manager. It blocks until ctx is cancelled.
// It should be started in a goroutine from main.
func (m *Manager) Run(ctx context.Context) {
	log.Printf("📡 Capture manager started")

	for {
		// Phase 1: discover which generation is active.
		gen := m.waitForGeneration(ctx)
		if ctx.Err() != nil {
			log.Printf("📡 Capture manager stopped (context cancelled during generation detection)")
			return
		}

		// Phase 2: discover the bridge interface.
		iface, err := m.discoverInterface(ctx)
		if err != nil {
			log.Printf("⚠️  Capture: interface discovery failed: %v — retrying in %s", err, generationPollInterval)
			select {
			case <-time.After(generationPollInterval):
				continue
			case <-ctx.Done():
				return
			}
		}

		m.mu.Lock()
		m.iface = iface
		m.generation = gen
		m.startTime = time.Now()
		m.mu.Unlock()

		log.Printf("📡 Capture ready: iface=%s generation=%s", iface, gen)

		// Phase 3: run tshark, restarting on failure with backoff.
		m.runWithRestart(ctx, iface, gen)

		if ctx.Err() != nil {
			log.Printf("📡 Capture manager stopped")
			return
		}

		// If we get here the context is still alive but the generation may have
		// changed (e.g. operator switched from 5G to 4G core). Reset and re-detect.
		log.Printf("📡 Capture loop exited — re-detecting generation")
		m.mu.Lock()
		m.iface = ""
		m.generation = ""
		m.mu.Unlock()
	}
}

// waitForGeneration polls the collector snapshot until a single active
// generation is detected among running core containers.
func (m *Manager) waitForGeneration(ctx context.Context) string {
	for {
		gen := m.snap.ActiveGeneration()
		if gen != "" {
			log.Printf("📡 Generation detected: %s", gen)
			return gen
		}

		log.Printf("📡 No active core generation detected — waiting %s", generationPollInterval)
		select {
		case <-time.After(generationPollInterval):
		case <-ctx.Done():
			return ""
		}
	}
}

// discoverInterface returns the bridge interface name, either from dynamic
// Docker network inspection or from the explicitly configured value.
func (m *Manager) discoverInterface(ctx context.Context) (string, error) {
	if m.captureInterface != "" && m.captureInterface != "auto" {
		log.Printf("📡 Using configured capture interface: %s", m.captureInterface)
		return m.captureInterface, nil
	}
	return m.docker.GetBridgeInterface(ctx, networkName)
}

// runWithRestart runs two tshark subprocesses (SCTP and UDP) and restarts
// both on failure using exponential backoff. Returns when ctx is cancelled.
func (m *Manager) runWithRestart(ctx context.Context, iface, gen string) {
	backoff := restartBackoffInitial

	for {
		f := filtersFor(gen)

		// Launch SCTP subprocess (S1AP / NGAP)
		sctpPkts, sctpErrc := startTshark(ctx, iface, f.SCTPBPF, f.SCTPDisplay)

		// Launch UDP subprocess (GTPv2 / PFCP)
		udpPkts, udpErrc := startTshark(ctx, iface, f.UDPBPF, f.UDPDisplay)

		// Merge both packet channels into the manager output channel
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case pkt, ok := <-sctpPkts:
					if !ok {
						sctpPkts = nil
					} else {
						switch pkt.Generation {
						case Generation4G:
							m.packets4g.Add(1)
						case Generation5G:
							m.packets5g.Add(1)
						}
						select {
						case m.out <- pkt:
						case <-ctx.Done():
							return
						}
					}
				case pkt, ok := <-udpPkts:
					if !ok {
						udpPkts = nil
					} else {
						switch pkt.Generation {
						case Generation4G:
							m.packets4g.Add(1)
						case Generation5G:
							m.packets5g.Add(1)
						}
						select {
						case m.out <- pkt:
						case <-ctx.Done():
							return
						}
					}
				case <-ctx.Done():
					return
				}
				if sctpPkts == nil && udpPkts == nil {
					return
				}
			}
		}()

		// Wait for either subprocess to exit
		var exitErr error
		select {
		case err := <-sctpErrc:
			if ctx.Err() == nil {
				exitErr = err
			}
		case err := <-udpErrc:
			if ctx.Err() == nil {
				exitErr = err
			}
		}
		<-done

		if ctx.Err() != nil {
			return
		}

		m.restarts.Add(1)
		log.Printf("⚠️  tshark exited unexpectedly (restart #%d): %v — retrying in %s",
			m.restarts.Load(), exitErr, backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		backoff *= 2
		if backoff > restartBackoffMax {
			backoff = restartBackoffMax
		}
	}
}
