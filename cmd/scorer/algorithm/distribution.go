package algorithm

import (
	"math"
)

type Distribution struct {
	name        string
	normalizeFn func(float64) float64
}

func (d *Distribution) String() string {
	return d.name
}

func (d *Distribution) Normalize(v float64) float64 {
	return d.normalizeFn(v)
}

var (
	normalizationFuncs = map[string]func(float64) float64{
		"linear":  func(v float64) float64 { return v },
		"zapfian": func(v float64) float64 { return math.Log(1 + v) },
	}
	DefaultDistributionName = "linear"
)

func LookupDistribution(name string) *Distribution {
	fn, ok := normalizationFuncs[name]
	if !ok {
		return nil
	}
	return &Distribution{
		name:        name,
		normalizeFn: fn,
	}
}
