package css

const (
	MaxStylesheetRules          = 100_000
	MaxSelectorsPerRule         = 256
	MaxSelectorCombinators      = 64
	MaxFunctionalSelectorDepth  = 32
	MaxCascadeLayers            = 256
	MaxCustomProperties         = 4_096
	MaxCustomPropertyValueBytes = 64 << 10
	MaxCSSFunctionDepth         = 64
)
