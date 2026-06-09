// Package latency drives the toxiproxy control plane to apply / clear
// latency presets between the benchmark client and postgres.
//
// The toxic configuration is symmetric: we apply latency toxics to both
// the upstream and downstream legs of the proxy so total RTT = 2 ×
// one-way delay. Toxiproxy speaks integer milliseconds, so very small
// presets (intradc) round to the millisecond floor.
package latency

import (
	"errors"
	"fmt"
	"time"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
)

// Preset is the named latency profile applied to the toxiproxy proxy.
type Preset string

const (
	PresetNone             Preset = "none"
	PresetIntraDC          Preset = "intradc"
	PresetCrossAZ          Preset = "crossaz"
	PresetCrossRegion      Preset = "crossregion"
	PresetIntercontinental Preset = "intercontinental"
)

// AllPresets is the ordered list used by --sweep-latency.
var AllPresets = []Preset{
	PresetNone, PresetIntraDC, PresetCrossAZ, PresetCrossRegion, PresetIntercontinental,
}

// Profile is the (one-way delay, jitter) parameter pair for a preset.
// RTT = 2 × OneWay (jitter applied symmetrically). Stored as
// time.Duration; toxiproxy itself takes integer milliseconds.
type Profile struct {
	OneWay time.Duration
	Jitter time.Duration // peak deviation, applied to each leg
}

// Profiles maps each preset to its delay parameters. Values are rounded
// to the millisecond floor when handed to toxiproxy, so PresetIntraDC's
// 1ms target ends up as 1ms ± 0 (toxiproxy can't express sub-ms).
var Profiles = map[Preset]Profile{
	PresetNone:             {0, 0},
	PresetIntraDC:          {1 * time.Millisecond, 0},
	PresetCrossAZ:          {2 * time.Millisecond, 0},
	PresetCrossRegion:      {15 * time.Millisecond, 1 * time.Millisecond},
	PresetIntercontinental: {50 * time.Millisecond, 5 * time.Millisecond},
}

// IsValid reports whether p is a known preset.
func (p Preset) IsValid() bool {
	_, ok := Profiles[p]
	return ok
}

const (
	upstreamToxicName   = "otelbench_upstream_latency"
	downstreamToxicName = "otelbench_downstream_latency"
)

// Client manages the toxics attached to a single named toxiproxy proxy.
type Client struct {
	c     *toxiproxy.Client
	proxy *toxiproxy.Proxy
	name  string
}

// New constructs a Client and fetches the named proxy. Returns an error
// if the proxy doesn't exist --- the docker-compose / dev setup is
// responsible for creating it; the benchmark just attaches toxics to
// an already-configured proxy.
func New(apiURL, proxyName string) (*Client, error) {
	c := toxiproxy.NewClient(apiURL)
	p, err := c.Proxy(proxyName)
	if err != nil {
		return nil, fmt.Errorf("toxiproxy proxy %q: %w (is toxiproxy running at %s with the proxy configured?)",
			proxyName, err, apiURL)
	}
	return &Client{c: c, proxy: p, name: proxyName}, nil
}

// Apply replaces any existing otelbench toxics with the pair matching p.
// Idempotent --- safe to call repeatedly between cells in a sweep.
func (c *Client) Apply(p Preset) error {
	prof, ok := Profiles[p]
	if !ok {
		return errors.New("unknown latency preset: " + string(p))
	}
	if err := c.Clear(); err != nil {
		return err
	}
	if p == PresetNone {
		return nil // no toxics for passthrough; reports the floor latency
	}
	latencyMs := int(prof.OneWay / time.Millisecond)
	jitterMs := int(prof.Jitter / time.Millisecond)
	if _, err := c.proxy.AddToxic(upstreamToxicName, "latency", "upstream", 1.0, toxiproxy.Attributes{
		"latency": latencyMs,
		"jitter":  jitterMs,
	}); err != nil {
		return fmt.Errorf("add upstream latency toxic: %w", err)
	}
	if _, err := c.proxy.AddToxic(downstreamToxicName, "latency", "downstream", 1.0, toxiproxy.Attributes{
		"latency": latencyMs,
		"jitter":  jitterMs,
	}); err != nil {
		return fmt.Errorf("add downstream latency toxic: %w", err)
	}
	return nil
}

// Clear removes any otelbench-managed toxics from the proxy. Toxics not
// added by us (e.g. a manual debugging toxic the user added via the
// toxiproxy CLI) are left in place.
func (c *Client) Clear() error {
	toxics, err := c.proxy.Toxics()
	if err != nil {
		return fmt.Errorf("list toxics: %w", err)
	}
	for _, t := range toxics {
		if t.Name == upstreamToxicName || t.Name == downstreamToxicName {
			if err := c.proxy.RemoveToxic(t.Name); err != nil {
				return fmt.Errorf("remove toxic %s: %w", t.Name, err)
			}
		}
	}
	return nil
}
