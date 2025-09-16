package main

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"wt-datalogging/internal/tracks"

	"github.com/stratoberry/go-gpsd"
	"go.einride.tech/can/pkg/socketcan"
)

// --- MISC ---
const SETTINGSFILEPATH = "wt-app-settings.json"

// --- Data conversion constants ---
// Oil Temp
const OILTEMP_A = 0.0014222095
const OILTEMP_B = 0.00023729017
const OILTEMP_C = 9.3273998e-8

// Oil Pressure
const OILPRESSURE_originalLow float64 = 0    //0.5
const OILPRESSURE_originalHigh float64 = 5   //4.5
const OILPRESSURE_desiredLow float64 = -100  //0
const OILPRESSURE_desiredHigh float64 = 1100 //1000

type AppSettings struct {
	LoggingHertz int    `json:"loggingHertz"`
	CanChannel   string `json:"canChannel"`
	Car          string `json:"car"`
	Ecu          string `json:"ecu"`
	Track        string `json:"track"`
	LapTiming    bool   `json:"lapTiming"`
}

type GpsData struct {
	PreviousLat float64
	PreviousLon float64
	TypeTrigger bool
}

type LapTiming struct {
	SessionTimeMs     uint32   // absolute session time in ms, safe up to 49 days
	LapIndex          uint16   // lap counter, safe up to 65k laps
	LapStartTimeMs    uint32   // absolute session time at last lap start
	// SectorStartTimeMs []uint32 // absolute session time when each sector is started
	// Flags             uint8    // metadata (pits, invalid lap, yellow flag, etc)
}

var (
	appSettings  *AppSettings
	currentTrack tracks.Track

	localRpm               uint16
	localSpeed             uint16
	localGear              uint8
	localVoltage           float64
	localIat               uint16
	localEct               uint16
	localMil               uint8
	localVts               uint8
	localCl                uint8
	localTps               uint16
	localMap               uint16
	localInj               uint16
	localIgn               uint16
	localLambdaRatio       float64
	localKnockCounter      uint16
	localTargetCamAngle    float64
	localActualCamAngle    float64
	localAnalog0           uint16 // Oil Temperature
	localAnalog1           uint16 // Oil Pressure
	localAnalog2           uint16
	localAnalog3           uint16
	localAnalog4           uint16
	localAnalog5           uint16
	localAnalog6           uint16
	localAnalog7           uint16
	localEthanolInput1     uint8
	localEthanolInput2S300 float64
	localEthanolInput2KPro uint8
	localEthanolInput3     uint16

	localLat               float64
	localLon               float64
	localSessionTimeMs     uint32
	localLapIndex          uint16
	localLapStartTimsMs    uint32
	// localSectorStartTimeMs []uint32
	// localFlags             uint8

	gpsData = GpsData{}
	lapTiming = LapTiming{ LapIndex: 0, Flags: 0}
)

func DataLoggingAtSpecificHertz(w *csv.Writer) {
	startTimeStamp := []string{time.Now().Format("02-01-2006 - 15:04:05"), appSettings.Track, appSettings.Car, appSettings.Ecu}
	if err := w.Write(startTimeStamp); err != nil {
		log.Fatalln("Error writing datalogging start timestamp CSV")
	}

	csvHeaders := []string{"HertzTime", "Engine RPM", "Speed", "Gear", "Voltage", "IAT", "ECT", "MIL", "VTS", "CL", "TPS", "MAP", "INJ", "IGN", "Lambda Ratio", "Knock Count", "Target Cam Angle", "Actual Cam Angle", "Analog0", "Analog1", "Analog2", "Analog3", "Analog4", "Analog5", "Analog6", "Analog7", "Ethanol Input1", "Ethanol Input2", "Ethanol Input3", "Latitude", "Longitude", "SessionTimeMs", "LapIndex", "LapStartTimeMs"} //, "SectorStartTimeMs1", "SectorStartTimeMs2", "SectorStartTimeMs3", "Flags"}
	csvHeaderTypes := []string{"sec", "rpm", "km/h", "", "V", "C", "C", "", "", "", "%", "kPa", "ms", "deg", "lambda", "count", "deg", "deg", "", "", "", "", "", "", "", "", "hz", "%", "%", "deg", "deg", "ms", "int", "ms"} //, "ms", "ms", "ms", "int"}
	if err := w.Write(csvHeaders); err != nil {
		log.Fatalln("Error writing headers to CSV")
	}
	if err := w.Write(csvHeaderTypes); err != nil {
		log.Fatalln("Error writing header types to CSV")
	}

	startTime := time.Now()
	ticker := time.NewTicker(time.Second / time.Duration(appSettings.LoggingHertz))
	defer ticker.Stop()

	/* How the Hertz is calc'd
	 * - The NEW way takes the `startTime` and compares it against the `currentTime` each tick
	 * - Then work out the seconds and fraction of the `elapsedTime`
	 * - Then format as desired ("00.0, 00.1, 00.2")
	 */
	for range ticker.C {
		// Hertz calculation
		currentTime := time.Now()
		elapsed := currentTime.Sub(startTime).Milliseconds()
		seconds := elapsed / 1000
		fraction := (elapsed % 1000) / 100
		time := fmt.Sprintf("%02d.%01d", seconds, fraction)

		var ethanolInput2 string
		if appSettings.Ecu == "s300" {
			ethanolInput2 = strconv.FormatFloat(float64(localEthanolInput2S300), 'f', 2, 64)
		} else {
			ethanolInput2 = strconv.FormatUint(uint64(localEthanolInput2KPro), 10)
		}

		csvFrame := []string{
			time,
			strconv.FormatUint(uint64(localRpm), 10),
			strconv.FormatUint(uint64(localSpeed), 10),
			strconv.FormatUint(uint64(localGear), 10),
			strconv.FormatFloat(float64(localVoltage), 'f', 1, 64),
			strconv.FormatUint(uint64(localIat), 10),
			strconv.FormatUint(uint64(localEct), 10),
			strconv.FormatUint(uint64(localMil), 10),
			strconv.FormatUint(uint64(localVts), 10),
			strconv.FormatUint(uint64(localCl), 10),
			strconv.FormatUint(uint64(localTps), 10),
			strconv.FormatUint(uint64(localMap), 10),
			strconv.FormatUint(uint64(localInj), 10),
			strconv.FormatUint(uint64(localIgn), 10),
			strconv.FormatFloat(float64(localLambdaRatio), 'f', 2, 64),
			strconv.FormatUint(uint64(localKnockCounter), 10),
			strconv.FormatFloat(float64(localTargetCamAngle), 'f', 2, 64),
			strconv.FormatFloat(float64(localActualCamAngle), 'f', 2, 64),
			strconv.FormatUint(uint64(localAnalog0), 10),
			strconv.FormatUint(uint64(localAnalog1), 10),
			strconv.FormatUint(uint64(localAnalog2), 10),
			strconv.FormatUint(uint64(localAnalog3), 10),
			strconv.FormatUint(uint64(localAnalog4), 10),
			strconv.FormatUint(uint64(localAnalog5), 10),
			strconv.FormatUint(uint64(localAnalog6), 10),
			strconv.FormatUint(uint64(localAnalog7), 10),
			strconv.FormatUint(uint64(localEthanolInput1), 10),
			ethanolInput2,
			strconv.FormatUint(uint64(localEthanolInput3), 10),
			strconv.FormatFloat(float64(localLat), 'f', 10, 64),
			strconv.FormatFloat(float64(localLon), 'f', 10, 64),
			strconv.FormatUint(uint64(localSessionTimeMs), 10),
			strconv.FormatUint(uint64(localLapIndex), 10),
			strconv.FormatUint(uint64(localLapStartTimsMs), 10),
			// strconv.FormatUint(uint64(localSectorStartTimeMs[0]), 10),
			// strconv.FormatUint(uint64(localSectorStartTimeMs[1]), 10),
			// strconv.FormatUint(uint64(localSectorStartTimeMs[2]), 10),
			// strconv.FormatUint(uint64(localFlags), 10),
		}

		if err := w.Write(csvFrame); err != nil {
			log.Fatalln("Error writing data to CSV", err)
		}
	}
}

func isThisTheFinishLine(x3 float64, y3 float64, x4 float64, y4 float64) bool {
	x1 := currentTrack.LatMin
	y1 := currentTrack.LonMin
	x2 := currentTrack.LatMax
	y2 := currentTrack.LonMax

	// ** We calculate the intersection point on both the finish line AND movement line
	// - FinishLine = line across the track (min to max points)
	// - MovementPath = previous location to current location

	denominator := (x3 - x4) * (y1 - y2) - (y3 - y4) * (x1 - x2)

	// If denominator is 0, the lines are parallel or coincident
	if math.Abs(denominator) < 1e-10 {
		return false
	}

	// Calculate the numerators
	tNumerator := (x3 - x1) * (y1 - y2) - (y3 - y1) * (x1 - x2)
	uNumerator := (x3 - x1) * (y3 - y4) - (y3 - y1) * (x3 - x4)

	// t - Parametric value along the finish line segment
	// u - Parametric value along the movement path
	t := tNumerator / denominator
	u := uNumerator / denominator

	// Check if the intersection happens on both segments
	return (t >= 0 && t <= 1) && (u >= 0 && u <= 1)
}

func handleLapTiming() {
	// Connect to GPS server....
	// var gps *gpsd.Session
	// var err error

	// // Connect to the GPSD server
	// for {
	// 	gps, err = gpsd.Dial("localhost:2947")
	// 	if err != nil {
	// 		fmt.Println("Failed to connect to GPSD: ", err)
	// 		fmt.Println("Retrying in 10 seconds...")
	// 		time.Sleep(10 * time.Second)
	// 		continue
	// 	}

	// 	fmt.Println("Connected to GPSD")
	// 	break
	// }
	// defer gps.Close()
	
	
	// Start session timer
	sessionStart := time.Now()
	go func() {
		for {
			currentSessionTime := uint32(time.Since(sessionStart).Milliseconds())
			lapTiming.SessionTimeMs = currentSessionTime
			localSessionTimeMs = currentSessionTime
		}
	}()


	// tpvFilter := func(r interface{}) {
	// 	report := r.(*gpsd.TPVReport)
	for {
		currentTimeMs := lapTiming.SessionTimeMs //- lapTiming.LapStartTimeMs
		fmt.Println("Current: ", currentTimeMs)

		//if isThisTheFinishLine(gpsData.PreviousLat, gpsData.PreviousLon, report.Lat, report.Lon) {
		if gpsData.TypeTrigger {
			lapTiming.LapStartTimeMs = lapTiming.SessionTimeMs
			lapTiming.LapIndex++
			localLapStartTimsMs = lapTiming.LapStartTimeMs
			localLapIndex = lapTiming.LapIndex
			
			gpsData.TypeTrigger = false
		}

		// gpsData.PreviousLat = report.Lat
		// gpsData.PreviousLon = report.Lon
		// localLat = report.Lat
		// localLon = report.Lon
	}

	// gps.AddFilter("TPV", tpvFilter)
	// done := gps.Watch()
	// <-done
	// gps.Close()
}

func handleGpsDatalogging() {
	var gps *gpsd.Session
	var err error

	// Connect to the GPSD server
	for {
		gps, err = gpsd.Dial("localhost:2947")
		if err != nil {
			fmt.Println("Failed to connect to GPSD: ", err)
			fmt.Println("Retrying in 10 seconds...")
			time.Sleep(10 * time.Second)
			continue
		}

		fmt.Println("Connected to GPSD")
		break
	}
	defer gps.Close()

	sessionStart := time.Now().Round(100 * time.Millisecond)

	// Define a reporting filter
	tpvFilter := func(r interface{}) {
		report := r.(*gpsd.TPVReport)

		// ----- Convert report.Time from UTC to Australia/Sydney -----
		location, err := time.LoadLocation("Australia/Sydney")
		if err != nil {
			fmt.Println("Error loading location:", err)
			return
		}
		convertedCurrentTime := report.Time.In(location)

		// ---------- GPS/Lap Timing ----------
		timeDiff := convertedCurrentTime.Sub(sessionStart)
		// currentLapData.CurrentLapTime = uint32(timeDiff.Milliseconds())
		lapTiming.SessionTimeMs = uint32(timeDiff.Milliseconds())
		

		if isThisTheFinishLine(gpsData.PreviousLat, gpsData.PreviousLon, report.Lat, report.Lon) {
			lapTiming.LapStartTimeMs = lapTiming.SessionTimeMs
			lapTiming.LapIndex++
			localLapStartTimsMs = lapTiming.LapStartTimeMs
			localLapIndex = lapTiming.LapIndex
		}

		gpsData.PreviousLat = report.Lat
		gpsData.PreviousLon = report.Lon
		localLat = report.Lat
		localLon = report.Lon
	}

	gps.AddFilter("TPV", tpvFilter)
	done := gps.Watch()
	<-done
	gps.Close()
}

func ReadWTSettingsFile() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("[FATAL] Cannot get CWD : ", err)
	}
	devRoot := filepath.Dir(cwd) // Go up 1 level

	settingsFile, err := os.Open(filepath.Join(devRoot, SETTINGSFILEPATH))
	if err != nil {
		log.Fatal("[FATAL] Cannot read in settings file : ", err)
	}
	defer settingsFile.Close()

	data, _ := io.ReadAll(settingsFile)
	json.Unmarshal(data, &appSettings)
	currentTrack = tracks.Tracks[appSettings.Track]
}

func main() {
	fmt.Println("--- Datalogging initialising... ---")
	ReadWTSettingsFile()

	// -------------------- Create CSV file and write headers to it --------------------
	// Count how many datalogs current exist, and increment the count for the new one
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("[FATAL] Cannot get CWD : ", err)
	}

	// Go up 1 level
	devRoot := filepath.Dir(cwd)
	// Path to "data" folder
	dataPath := filepath.Join(devRoot, "data")

	err = os.MkdirAll(dataPath, 0755)
	if err != nil {
		log.Fatal("[FATAL] Cannot create data directory : ", err)
	}

	// Wrap as FS
	root := os.DirFS(dataPath)
	// Find CSV files
	mdFiles, err := fs.Glob(root, "*.csv")
	counter := 1
	for range mdFiles {
		counter++
	}

	// Now do the file creation
	fullPath := filepath.Join(dataPath, "datalog" + strconv.Itoa(counter) + ".csv")
	file, err := os.Create(fullPath)
	if err != nil {
		log.Fatalf("[FATAL] Issue creating file : %v", err)
	}
	defer file.Close()

	// Create a CSV writer that writes directly to the file
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// -------------------- Read CAN and write to file --------------------
	conn, _ := socketcan.DialContext(context.Background(), "can", appSettings.CanChannel)
	defer conn.Close()
	recv := socketcan.NewReceiver(conn)

	// Do datalogging
	go DataLoggingAtSpecificHertz(writer)

	// Do GPS datalogging
	if appSettings.LapTiming {
		// go handleGpsDatalogging()
		go handleLapTiming()
	}

	for recv.Receive() {
		frame := recv.Frame()

		// Button input from user to stop the datalogging
		if frame.ID == uint32(104) {
			return
		}
		
		// Iterate over all the ID's now to match current message
		switch frame.ID {
		case 660, 1632:
			gpsData.TypeTrigger = true
			localRpm = binary.BigEndian.Uint16(frame.Data[0:2])
			localSpeed = binary.BigEndian.Uint16(frame.Data[2:4])
			localGear = frame.Data[4]
			localVoltage = float64(frame.Data[5]) / 10.0

		case 661, 1633:
			localIat = binary.BigEndian.Uint16(frame.Data[0:2])
			localEct = binary.BigEndian.Uint16(frame.Data[2:4])
			if appSettings.Ecu == "kpro" {
				localMil = frame.Data[4]
				localVts = frame.Data[5]
				localCl = frame.Data[6]
			}

		case 662, 1634:
			localTps = binary.BigEndian.Uint16(frame.Data[0:2])
			if localTps == 65535 {
				localTps = 0
			}
			localMap = binary.BigEndian.Uint16(frame.Data[2:4]) / 10

		case 663, 1635:
			localInj = binary.BigEndian.Uint16(frame.Data[0:2]) / 1000
			localIgn = binary.BigEndian.Uint16(frame.Data[2:4])

		case 664, 1636:
			localLambdaRatio = math.Round(float64(32768.0)/float64(binary.BigEndian.Uint16(frame.Data[0:2]))*100) / 100

		// K-Pro only
		case 665, 1637:
			if appSettings.Ecu == "kpro" {
				localKnockCounter = binary.BigEndian.Uint16(frame.Data[0:2])
			}

		// K-Pro only
		case 666, 1638:
			if appSettings.Ecu == "kpro" {
				localTargetCamAngle = float64(binary.BigEndian.Uint16(frame.Data[0:2]))
				localActualCamAngle = float64(binary.BigEndian.Uint16(frame.Data[2:4]))
			}

		case 667, 1639:
			// Oil Temp
			oilTempResistance := binary.BigEndian.Uint16(frame.Data[0:2])
			kelvinTemp := 1 / (OILTEMP_A + OILTEMP_B * math.Log(float64(oilTempResistance)) + OILTEMP_C * math.Pow(math.Log(float64(oilTempResistance)), 3))
			localAnalog0 = uint16(kelvinTemp - 273.15)

			// Oil Pressure
			oilPressureResistance := float64(binary.BigEndian.Uint16(frame.Data[2:4])) / 819.2
			kPaValue := ((float64(oilPressureResistance) - OILPRESSURE_originalLow) / (OILPRESSURE_originalHigh - OILPRESSURE_originalLow) * (OILPRESSURE_desiredHigh - OILPRESSURE_desiredLow)) + OILPRESSURE_desiredLow
			localAnalog1 = uint16(math.Round(kPaValue * 0.145038)) // Convert to psi
			localAnalog2 = binary.BigEndian.Uint16(frame.Data[4:6])
			localAnalog3 = binary.BigEndian.Uint16(frame.Data[6:8])

		case 668, 1640:
			localAnalog4 = binary.BigEndian.Uint16(frame.Data[0:2])
			localAnalog5 = binary.BigEndian.Uint16(frame.Data[2:4])
			localAnalog6 = binary.BigEndian.Uint16(frame.Data[4:6])
			localAnalog7 = binary.BigEndian.Uint16(frame.Data[6:8])

		case 669, 1641:
			localEthanolInput1 = frame.Data[0]

			if appSettings.Ecu == "s300" {
				localEthanolInput2S300 = float64(frame.Data[1]) * 2.56 // Duty
				localEthanolInput3 = uint16(frame.Data[2])             // Ethanol Content
			} else if appSettings.Ecu == "kpro" {
				localEthanolInput2KPro = frame.Data[1]                        // Ethanol Content
				localEthanolInput3 = binary.BigEndian.Uint16(frame.Data[2:4]) // Fuel Temperature
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatal(err)
	}

	log.Println("CSV file 'datalog.csv' has been created successfully.")
}
