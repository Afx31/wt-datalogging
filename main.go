package main

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
	"math"

	"go.einride.tech/can/pkg/socketcan"
)

/// --- Local variables to write to, which the datalogging will snapshot later ---
var localRpm uint16
var localSpeed uint16
var localGear uint8
var localVoltage uint8
var localIat uint16
var localEct uint16
var localTps uint16
var localMap uint16
var localLambdaRatio uint16
var localOilTemp uint16
var localOilPressure uint16

func DataLoggingAtSpecificHertz(ticker *time.Ticker, quit chan struct{}, w *csv.Writer) {
	csvHeaders := []string{"Time","Engine RPM","Speed","Gear","Voltage","IAT","ECT","TPS","MAP","Lambda Ratio","Oil Temperature","Oil Pressure"}
	csvHeaderTypes := []string{"sec","rpm","kmh","int","v","c","c","int","kpa","int","c","p"}
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
			time := fmt.Sprintf("%02d.%02d", int(elapsed.Seconds()), counter)
			
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

func main() {
	// Config, move to a config file later
	configCanDevice := "vcan0"
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
	file, err := os.Create("data.csv")
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

	for recv.Receive() {
		frame := recv.Frame()
		
		// Button input from user to stop the datalogging
		if frame.ID == configStopDataloggingId {
			return
		}
		
		// Iterate over all the ID's now to match current message
		switch frame.ID {
		case 660:
		// case 1632:
			localRpm = binary.BigEndian.Uint16(frame.Data[0:2])
			localSpeed = binary.BigEndian.Uint16(frame.Data[2:4])
			localGear = frame.Data[4]
			localVoltage = frame.Data[5] / 10
		case 661:
		// case 1633:
			localIat = binary.BigEndian.Uint16(frame.Data[0:2])
			localEct = binary.BigEndian.Uint16(frame.Data[2:4])
		case 662:
		// case 1634:
			localTps = binary.BigEndian.Uint16(frame.Data[0:2])
			localMap = binary.BigEndian.Uint16(frame.Data[2:4]) / 10
		case 664:
		// case 1636:
			localLambdaRatio = 32768 / binary.BigEndian.Uint16(frame.Data[0:2])
		case 667:
		// case 1639:
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
