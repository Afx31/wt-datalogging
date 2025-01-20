package tracks

type Track struct {
	LatMin, LatMax float64
	LonMin, LonMax float64
}

var Tracks = map[string]Track{
	"smsp": {
		LatMin: -33.803855,
		LatMax: -33.803649,
		LonMin: 150.870905,
		LonMax: 150.870954,
	},
	"morganpark": {
		LatMin: -28.262057,
		LatMax: -28.262087,
		LonMin: 152.036282,
		LonMax: 152.036477,
	},
	"winton": {
		LatMin: -28.262057,
		LatMax: -28.262087,
		LonMin: 152.036282,
		LonMax: 152.036477,
	},
}