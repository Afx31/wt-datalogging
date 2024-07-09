package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"go.einride.tech/can/pkg/socketcan"
)

func main() {
	// Temp config:
	configCanDevice := "vcan0"
	configStopDataloggingId := uint32(1911)

	fmt.Println("--- Datalogging initialising... ---")

	// -------------------- Write headers to CSV file first --------------------
	headers := "id,dlc,b0,b1,b2,b3,b4,b5,b6,b7"
	csvHeaders := strings.Split(headers, ",")
	//Create a new file to write CSV data to
	file, err := os.Create("data.csv")
	if err != nil {
		log.Fatalf("Error creating file: %v", err)
	}
	defer file.Close()

	// Create a CSV writer that writes directly to the file
	w := csv.NewWriter(file)
	defer w.Flush()

	if err := w.Write(csvHeaders); err != nil {
		log.Fatalln("Error writing headers to CSV", err)
	}


	// -------------------- Now do CAN stuff --------------------
	conn, _ := socketcan.DialContext(context.Background(), "can", configCanDevice)
	defer conn.Close()
	recv := socketcan.NewReceiver(conn)

	for recv.Receive() {
		frame := recv.Frame()
		
		// Button input from user to stop the datalogging
		if frame.ID == configStopDataloggingId {
			return
		}

		// if err := frame.Validate(); err != nil {
		// 	fmt.Println("Error validating frame:", err)
		// }

		var hexData []string
		for i := 0; i < int(frame.Length); i++ {
			hexData = append(hexData, fmt.Sprintf("%02X", frame.Data[i]))
		}
		
		csvFrame := append([]string{
			strconv.FormatUint(uint64(frame.ID), 10),
			strconv.FormatUint(uint64(frame.Length), 10),
		}, hexData...)

		if err := w.Write(csvFrame); err != nil {
			log.Fatalln("Error writing headers to CSV", err)
		}
	}

	// Flush any buffered data to ensure all data is written to the file
	w.Flush()

	if err := w.Error(); err != nil {
		log.Fatal(err)
	}

	log.Println("CSV file 'data.csv' has been created successfully.")
}