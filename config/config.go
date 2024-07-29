package config

type PIDConfig struct {
	Id 			int
	Offset 	int
	Size 		int	
}

func GetCanPIDConfig() map[string]map[string]PIDConfig {
	return map[string]map[string]PIDConfig{
		"honda": {
			"changeDisplay": {Id: 273, Offset: 0, Size: 1},
			"rpm": {Id: 1632, Offset: 0, Size: 2 },
			"speed": {Id: 1632, Offset: 2, Size: 2 },
			"gear": {Id: 1632, Offset: 4, Size: 1 },
			"voltage": {Id: 1632, Offset: 5, Size: 1 },
			"iat": {Id: 1633, Offset: 0, Size: 2 },
			"ect": {Id: 1633, Offset: 2, Size: 2 },
			"tps": {Id: 1634, Offset: 0, Size: 2 },
			"map": {Id: 1634, Offset: 2, Size: 2 },
			"inj": {Id: 1635, Offset: 0, Size: 2 },
			"ign": {Id: 1635, Offset: 2, Size: 2 },
			"lambdaRatio": {Id: 1636, Offset: 0, Size: 2 },
			"lambda": {Id: 1636, Offset: 2, Size: 2 },
			"oilTemp": {Id: 1639, Offset: 0, Size: 2 },
			"oilPressure": {Id: 1639, Offset: 2, Size: 2 },
		},
		"mazda": {
			"tps": {Id: 513, Offset: 6, Size: 1},
		},
	}
}