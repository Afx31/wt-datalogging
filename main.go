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
	
	for {
		select {
		case t := <-ticker.C:
			time := fmt.Sprintf("%02d.%02d", t.Second(), t.Nanosecond()/1e6)
			fmt.Println(time)
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
		// Rules:
			// If we need 2 bytes, we use `binary.BigEndian.Uint16() as it expects 2 bytes`
			// IF we need 1 byte, we just shove into a Uint8
		switch frame.ID {
		case 660:
			localRpm = binary.BigEndian.Uint16(frame.Data[0:2])
			localSpeed = binary.BigEndian.Uint16(frame.Data[2:4])
			localGear = frame.Data[4]
			localVoltage = frame.Data[5]
		case 661:
			localIat = binary.BigEndian.Uint16(frame.Data[0:2])
			localEct = binary.BigEndian.Uint16(frame.Data[2:4])
		case 662:
			localTps = binary.BigEndian.Uint16(frame.Data[0:2])
			localMap = binary.BigEndian.Uint16(frame.Data[2:4])
		case 664:
			localLambdaRatio = binary.BigEndian.Uint16(frame.Data[0:2])
		case 667:
			// Oil Temp
			oilTempResistance := binary.BigEndian.Uint16(frame.Data[0:2])
			kelvinTemp := 1 / (A + B * math.Log(float64(oilTempResistance)) + C * math.Pow(math.Log(float64(oilTempResistance)), 3))
			localOilTemp = uint16(kelvinTemp - 273.15)

			// Oil Pressure
			oilPressureResistance := binary.BigEndian.Uint16(frame.Data[2:4])
			kPaValue := ((float64(oilPressureResistance) - originalLow) / (originalHigh - originalLow) * (desiredHigh - desiredLow)) + desiredLow
			localOilPressure = uint16(math.Round(kPaValue * 0.145038)) // Convert to psi
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatal(err)
	}
}





// 	// -------------------- Read CAN and write to file --------------------
// 	conn, _ := socketcan.DialContext(context.Background(), "can", configCanDevice)
// 	defer conn.Close()
// 	recv := socketcan.NewReceiver(conn)

// 	for recv.Receive() {
// 		frame := recv.Frame()
		
// 		// Button input from user to stop the datalogging
// 		if frame.ID == configStopDataloggingId {
// 			return
// 		}

// 		// if err := frame.Validate(); err != nil {
// 		// 	fmt.Println("Error validating frame:", err)
// 		// }

// 		var hexData []string
// 		for i := 0; i < int(frame.Length); i++ {
// 			hexData = append(hexData, fmt.Sprintf("%02X", frame.Data[i]))
// 		}
		
// 		csvFrame := append([]string{
// 			strconv.FormatUint(uint64(frame.ID), 10),
// 			strconv.FormatUint(uint64(frame.Length), 10),
// 		}, hexData...)

// 		if err := w.Write(csvFrame); err != nil {
// 			log.Fatalln("Error writing headers to CSV", err)
// 		}
// 	}

// 	// Flush any buffered data to ensure all data is written to the file
// 	w.Flush()

// 	if err := w.Error(); err != nil {
// 		log.Fatal(err)
// 	}

// 	log.Println("CSV file 'data.csv' has been created successfully.")
// }
