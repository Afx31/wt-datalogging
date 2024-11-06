package main

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"strconv"
	"time"
	"wt-datalogging/internal/tracks"

	"github.com/stratoberry/go-gpsd"
	"go.einride.tech/can/pkg/socketcan"
)

/// --- Local variables to write to, which the datalogging will snapshot later ---
type LapStats struct {
  Type int8
  LapCount int8
	BestLapTime int32
	PbLapTime int32
	PreviousLapTime int32
}

var (
	localRpm uint16
	localSpeed uint16
	localGear uint8
	localVoltage uint8
	localIat uint16
	localEct uint16
	localTps uint16
	localMap uint16
	localLambdaRatio uint16
	localOilTemp uint16
	localOilPressure uint16

	localLat float64
	localLon float64
	localTime time.Time
	localLapStartTime time.Time
	localCurrentLapTime int32
	localLapCount int8
	localBestLapTime int32
	localPbLapTime int32
	localPreviousLapTime int32

	lapStats = LapStats{Type: 3, LapCount: 1}
)

type CurrentLapData struct {
	Type int8
  LapStartTime time.Time
	CurrentLapTime int32
}

func DataLoggingAtSpecificHertz(ticker *time.Ticker, quit chan struct{}, w *csv.Writer) {
	startTimeStamp := []string{ time.Now().Format("02-01-2006 - 15:04:05")}
	if  err := w.Write(startTimeStamp); err != nil {
		log.Fatalln("Error writing datalogging start timestamp CSV")
	}

	csvHeaders := []string{"HertzTime","Engine RPM","Speed","Gear","Voltage","IAT","ECT","TPS","MAP","Lambda Ratio","Oil Temperature","Oil Pressure","Latitude","Longitude","LapCount","CurrentTime","CurrentLapStartTime","CurrentLapTime","BestLapTime","PbLapTime","PreviousLapTime"}
	csvHeaderTypes := []string{"sec","rpm","kmh","int","v","c","c","int","kpa","int","c","p","int","int","int","time","time","time","time","time","time"}
	if err := w.Write(csvHeaders); err != nil {
		log.Fatalln("Error writing headers to CSV")
	}
	if err := w.Write(csvHeaderTypes); err != nil {
		log.Fatalln("Error writing header types to CSV")
	}
	
	startTime := time.Now()
	counter := 0
	for {
		select {
		case t := <-ticker.C:
			// Calc elapsed time from the start time, before proceeding
			elapsed := t.Sub(startTime)
			time := fmt.Sprintf("%02d.%01d", int(elapsed.Seconds()), counter)
			formattedLocalTime := localTime.Format("15:04:05 02-01-2006")
			formattedLapStartTime := localLapStartTime.Format("15:04:05 02-01-2006")
			
			csvFrame := append([]string{
				time,
				strconv.FormatUint(uint64(localRpm), 10),
				strconv.FormatUint(uint64(localSpeed), 10),
				strconv.FormatUint(uint64(localGear), 10),
				strconv.FormatUint(uint64(localVoltage), 10),
				strconv.FormatUint(uint64(localIat), 10),
				strconv.FormatUint(uint64(localEct), 10),
				strconv.FormatUint(uint64(localTps), 10),
				strconv.FormatUint(uint64(localMap), 10),
				strconv.FormatUint(uint64(localLambdaRatio), 10),
				strconv.FormatUint(uint64(localOilTemp), 10),
				strconv.FormatUint(uint64(localOilPressure), 10),
				strconv.FormatFloat(float64(localLat), 'f', 10, 64),
				strconv.FormatFloat(float64(localLon), 'f', 10, 64),
				strconv.FormatInt(int64(localLapCount), 10),
				formattedLocalTime,
				formattedLapStartTime,
				strconv.FormatInt(int64(localCurrentLapTime), 10),
				strconv.FormatInt(int64(localBestLapTime), 10),
				strconv.FormatInt(int64(localPbLapTime), 10),
				strconv.FormatInt(int64(localPreviousLapTime), 10),
			})

			// Hacky, but it works
			if (counter == 9) {
				counter = 0
			} else {
				counter++
			}
			
			if err := w.Write(csvFrame); err != nil {
				log.Fatalln("Error writing data to CSV", err)
			}
		case <-quit:
			ticker.Stop()
			return
		}
	}	

	time.Sleep(3 * time.Second)
}


func isThisTheFinishLine(min float64, max float64, current float64) bool {
  return current >= min && current <= max
}

func handleGpsDatalogging() {
	// Connect to the GPSD server
	gps, err := gpsd.Dial("localhost:2947")
	if err != nil {
		fmt.Println("Failed to connect to GPSD: ", err)
		return
	}

	currentLapData := CurrentLapData{Type: 2}
  currentLapData.LapStartTime = time.Now().Round(100 * time.Millisecond)

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
    timeDiff := convertedCurrentTime.Sub(currentLapData.LapStartTime)
    currentLapData.CurrentLapTime = int32(timeDiff.Milliseconds())

    if isThisTheFinishLine(tracks.SMSPLatMin, tracks.SMSPLatMax, report.Lat) && isThisTheFinishLine(tracks.SMSPLonMin, tracks.SMSPLonMax, report.Lon) {
      if currentLapData.CurrentLapTime < lapStats.BestLapTime || lapStats.BestLapTime == 0 {
        lapStats.BestLapTime = currentLapData.CurrentLapTime
      }
      if currentLapData.CurrentLapTime < lapStats.PbLapTime || lapStats.PbLapTime == 0 {
        lapStats.PbLapTime = currentLapData.CurrentLapTime
      }
      lapStats.PreviousLapTime = currentLapData.CurrentLapTime
      
      // Start the next lap
      currentLapData.LapStartTime = convertedCurrentTime
      lapStats.LapCount++;

	    // --- Update local values for the datalog ---
			localLapCount = lapStats.LapCount
			localBestLapTime = lapStats.BestLapTime
			localPbLapTime = lapStats.PbLapTime
			localPreviousLapTime = lapStats.PreviousLapTime
    }

		// --- Update local values for the datalog ---
		localLat = report.Lat
		localLon = report.Lon
		localTime = convertedCurrentTime
		localLapStartTime = currentLapData.LapStartTime
		localCurrentLapTime = currentLapData.CurrentLapTime
	}
  
	gps.AddFilter("TPV", tpvFilter)
	done := gps.Watch()
	<-done
	gps.Close()
}

func main() {
	// Config, move to a config file later
	configCanDevice := "can0"
	configStopDataloggingId := uint32(105) //hex = 69
	// hertz options [200 = 5hz | 100 = 10hz | 50 = 20hz]
	configHertz := 100

	// --- Misc configure for oil values ---
	// Oil Temp
	A := 0.0014222095
	B := 0.00023729017
	C := 9.3273998E-8
	// Oil Pressure
	var originalLow float64 = 0 //0.5
	var originalHigh float64 = 5 //4.5
	var desiredLow float64 = -100 //0
	var desiredHigh float64 = 1100 //1000

	// Pauses the difference from the current time to the full second to then force the ticker to start from the full second
	now := time.Now()
	pauseDuration := time.Second - time.Duration(now.Nanosecond()) * time.Nanosecond
	time.Sleep(pauseDuration)
	
	duration := time.Duration(configHertz) * time.Millisecond
	ticker := time.NewTicker(duration) // Create a ticker that ticks every 100 milliseconds
	quit := make(chan struct{})// Channel to signal when to stop the ticker

	fmt.Println("--- Datalogging initialising... ---")


	// -------------------- Create CSV file and write headers to it --------------------
	// Count how many datalogs current exist, and increment the count for the new one
	dir := "/home/pi/dev/data"
	root := os.DirFS(dir)
	mdFiles, err := fs.Glob(root, "*.csv")
	
	counter := 1;
	for range mdFiles {
		counter++;
	}

	// Now do the file creation
	file, err := os.Create("/home/pi/dev/data/datalog" + strconv.Itoa(counter) + ".csv")
	if err != nil {
		log.Fatalf("Error creating file: %v", err)
	}
	defer file.Close()

	// Create a CSV writer that writes directly to the file
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// -------------------- Read CAN and write to file --------------------
	conn, _ := socketcan.DialContext(context.Background(), "can", configCanDevice)
	defer conn.Close()
	recv := socketcan.NewReceiver(conn)

	// Do datalogging
	go DataLoggingAtSpecificHertz(ticker, quit, writer)

	// Do GPS datalogging
	go handleGpsDatalogging()

	for recv.Receive() {
		frame := recv.Frame()
		
		// Button input from user to stop the datalogging
		if frame.ID == configStopDataloggingId {
			return
		}
		
		// Iterate over all the ID's now to match current message
		switch frame.ID {
		case 660, 1632:
			localRpm = binary.BigEndian.Uint16(frame.Data[0:2])
			localSpeed = binary.BigEndian.Uint16(frame.Data[2:4])
			localGear = frame.Data[4]
			localVoltage = frame.Data[5] / 10
		case 661, 1633:
			localIat = binary.BigEndian.Uint16(frame.Data[0:2])
			localEct = binary.BigEndian.Uint16(frame.Data[2:4])
		case 662, 1634:
			localTps = binary.BigEndian.Uint16(frame.Data[0:2])
			localMap = binary.BigEndian.Uint16(frame.Data[2:4]) / 10
		case 664, 1636:
			localLambdaRatio = 32768 / binary.BigEndian.Uint16(frame.Data[0:2])
		case 667, 1639:
			// Oil Temp
			oilTempResistance := binary.BigEndian.Uint16(frame.Data[0:2])
			kelvinTemp := 1 / (A + B * math.Log(float64(oilTempResistance)) + C * math.Pow(math.Log(float64(oilTempResistance)), 3))
			localOilTemp = uint16(kelvinTemp - 273.15)

			// Oil Pressure
			oilPressureResistance := float64(binary.BigEndian.Uint16(frame.Data[2:4])) / 819.2
			kPaValue := ((float64(oilPressureResistance) - originalLow) / (originalHigh - originalLow) * (desiredHigh - desiredLow)) + desiredLow
			localOilPressure = uint16(math.Round(kPaValue * 0.145038)) // Convert to psi
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatal(err)
	}

	log.Println("CSV file 'data.csv' has been created successfully.")
}
