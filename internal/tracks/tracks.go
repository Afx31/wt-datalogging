package tracks

type Track struct {
	LatMin, LatMax float64
	LonMin, LonMax float64
}

var Tracks = map[string]Track{
	"smsp": {
		LatMin: -33.803830,
		LatMax: -33.803653,
		LonMin: 150.870918,
		LonMax: 150.870962,
	},
	"morganpark": {
		LatMin: -28.262069,
		LatMax: -28.262085,
		LonMin: 152.036327,
		LonMax: 152.036430,
	},
}