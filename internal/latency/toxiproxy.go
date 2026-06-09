// Package latency drives the toxiproxy control plane to apply / clear
// latency presets between the benchmark client and postgres.
//
// The toxic configuration is symmetric: we apply latency toxics to both
// the upstream and downstream legs of the proxy so total RTT = 2 ×
// one-way delay. Both legs use --latency-jitter as a percentage of the
// one-way delay so jitter scales with RTT.
package latency

import (
	"errors"
	"time"
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

// Profile is the (one-way delay, jitter fraction) parameter pair for a
// preset. RTT = 2 × OneWay (jitter applied symmetrically).
type Profile struct {
	OneWay         time.Duration
	JitterFraction float64 // 0.1 = ±10%
}

// Profiles maps each preset to its delay parameters.
var Profiles = map[Preset]Profile{
	PresetNone:             {0, 0},
	PresetIntraDC:          {500 * time.Microsecond, 0.10},
	PresetCrossAZ:          {2500 * time.Microsecond, 0.10},
	PresetCrossRegion:      {15 * time.Millisecond, 0.10},
	PresetIntercontinental: {50 * time.Millisecond, 0.10},
}

// Client manages the toxics attached to a single named toxiproxy proxy.
//
// TODO: implement on top of github.com/Shopify/toxiproxy/v2/client.
// Operations needed:
//
//	Apply(p Preset)   --- clears existing toxics, then adds upstream +
//	                      downstream latency toxics matching Profile[p].
//	Clear()           --- removes all toxics from the proxy.
//	MeasureRTT(ctx)   --- opens one connection, runs SELECT 1 a few
//	                      times, returns median RTT including
//	                      toxiproxy's own per-hop overhead. Reported
//	                      alongside results so the floor is visible.
type Client struct {
	apiURL    string
	proxyName string
}

func New(apiURL, proxyName string) *Client {
	return &Client{apiURL: apiURL, proxyName: proxyName}
}

func (c *Client) Apply(p Preset) error {
	_ = c
	if _, ok := Profiles[p]; !ok {
		return errors.New("unknown latency preset: " + string(p))
	}
	return errors.New("latency.Client.Apply not yet implemented")
}

func (c *Client) Clear() error {
	return errors.New("latency.Client.Clear not yet implemented")
}
