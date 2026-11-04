package types

const (
	SupplierBillingModeToken      = "token"
	SupplierBillingModeFixed      = "fixed"
	SupplierBillingModeExpression = "tiered_expr"
)

// SupplierModelTariff is a complete procurement contract in real USD. Token
// lanes are USD per million tokens; PriceUSD is per PriceUnit. Nil means the
// source did not define a lane, while a pointer to zero explicitly makes it free.
type SupplierModelTariff struct {
	SupplierID      int      `json:"-"`
	ProfileID       int      `json:"-"`
	Revision        int64    `json:"-"`
	ModelName       string   `json:"model_name"`
	BillingMode     string   `json:"billing_mode"`
	Expression      string   `json:"expression,omitempty"`
	InputUSD        *float64 `json:"input_usd,omitempty"`
	OutputUSD       *float64 `json:"output_usd,omitempty"`
	CacheReadUSD    *float64 `json:"cache_read_usd,omitempty"`
	CacheWriteUSD   *float64 `json:"cache_write_usd,omitempty"`
	CacheWrite1hUSD *float64 `json:"cache_write_1h_usd,omitempty"`
	ImageUSD        *float64 `json:"image_usd,omitempty"`
	ImageOutputUSD  *float64 `json:"image_output_usd,omitempty"`
	AudioInputUSD   *float64 `json:"audio_input_usd,omitempty"`
	AudioOutputUSD  *float64 `json:"audio_output_usd,omitempty"`
	ReasoningUSD    *float64 `json:"reasoning_usd,omitempty"`
	PriceUSD        *float64 `json:"price_usd,omitempty"`
	PriceUnit       string   `json:"price_unit,omitempty"`
	// Tools contains USD per invocation, including image_generation:quality:size.
	Tools map[string]float64 `json:"tools,omitempty"`
}

func (tariff SupplierModelTariff) Clone() SupplierModelTariff {
	copyPrice := func(price *float64) *float64 {
		if price == nil {
			return nil
		}
		value := *price
		return &value
	}
	tariff.InputUSD = copyPrice(tariff.InputUSD)
	tariff.OutputUSD = copyPrice(tariff.OutputUSD)
	tariff.CacheReadUSD = copyPrice(tariff.CacheReadUSD)
	tariff.CacheWriteUSD = copyPrice(tariff.CacheWriteUSD)
	tariff.CacheWrite1hUSD = copyPrice(tariff.CacheWrite1hUSD)
	tariff.ImageUSD = copyPrice(tariff.ImageUSD)
	tariff.ImageOutputUSD = copyPrice(tariff.ImageOutputUSD)
	tariff.AudioInputUSD = copyPrice(tariff.AudioInputUSD)
	tariff.AudioOutputUSD = copyPrice(tariff.AudioOutputUSD)
	tariff.ReasoningUSD = copyPrice(tariff.ReasoningUSD)
	tariff.PriceUSD = copyPrice(tariff.PriceUSD)
	if tariff.Tools != nil {
		tools := make(map[string]float64, len(tariff.Tools))
		for key, value := range tariff.Tools {
			tools[key] = value
		}
		tariff.Tools = tools
	}
	return tariff
}
