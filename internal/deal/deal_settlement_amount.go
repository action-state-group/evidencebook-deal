package deal

import (
	"encoding/json"
	"math/big"

	"github.com/action-state-group/capsule-cli/settlement"
)

// settlementAmount is an amount of a settlement leg (received, receive_fee):
// atomic units of a CAIP-19 asset at a scale.
type settlementAmount struct {
	Value      string      `json:"value"`
	AssetCode  string      `json:"assetCode"`
	AssetScale json.Number `json:"assetScale"`
}

func (a settlementAmount) scale() (int64, bool) {
	n, err := a.AssetScale.Int64()
	return n, err == nil && n >= 0 && n <= 30
}

// name is what a person reads for the amount's asset: "msat" for Lightning
// (lightningBTC counted in millisatoshi), else a known asset's name. ok is
// false for an asset it does not know: an asset id is not read out.
func (a settlementAmount) name() (string, bool) {
	scale, ok := a.scale()
	if !ok {
		return "", false
	}
	if a.AssetCode == lightningBTC && scale == lightningBTCScale {
		return "msat", true
	}
	n := settlement.AssetName(a.AssetCode)
	return n, n != ""
}

// units is the amount's atomic units.
func (a settlementAmount) units() (*big.Int, bool) {
	return new(big.Int).SetString(a.Value, 10)
}

// settlementText reads atomic units of the named asset: whole msat, or a
// decimal amount at the asset's two display places.
func settlementText(v *big.Int, name string) string {
	if name == "msat" {
		return v.String() + " msat"
	}
	return new(big.Rat).SetFrac(v, big.NewInt(1_000_000)).FloatString(2) + " " + name
}
