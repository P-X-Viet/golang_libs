package main

import (
	"fmt"
	"log"
	"runtime"
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

func main() {
	// Connect to the database
	db, err := gorm.Open("postgres", "host=localhost user=postgres password=your_password dbname=your_db sslmode=disable")
	if err != nil {
		log.Fatalf("failed to connect to the database: %v", err)
	}
	defer db.Close()

	// Number of workers based on CPU cores
	numWorkers := runtime.NumCPU()
	log.Printf("Using %d workers", numWorkers)

	// Synchronization
	var wg sync.WaitGroup
	var stackMutex sync.Mutex
	processedStack := make(map[uint]bool) // Tracks processed IDs

	// Worker function
	worker := func(workerID int) {
		defer wg.Done()

		for {
			var ids []uint

			// Step 1: Fetch a group of IDs using CTE
			func() {
				stackMutex.Lock()
				defer stackMutex.Unlock()

				rows, err := db.Raw(`
					WITH id_group AS (
						SELECT id FROM a
						WHERE id NOT IN (SELECT unnest(array[?]))
						ORDER BY id LIMIT 10
					)
					SELECT id FROM table
				`, keys(processedStack)).Rows()
				if err != nil {
					log.Printf("Worker %d: Error fetching ID group: %v", workerID, err)
					return
				}
				defer rows.Close()

				for rows.Next() {
					var id uint
					rows.Scan(&id)
					ids = append(ids, id)
				}
			}()

			// Exit if no IDs are left
			if len(ids) == 0 {
				break
			}

			// Step 2: Process the selected IDs using a CTE
			var resultData []struct {
				ID             uint
				ProcessedField string
			}
			err := db.Raw(`
				WITH tmp AS (
					SELECT id 
					FROM table
					WHERE st_intersects((select geom from table where gid = ?),geom ) AND some_condition = true
				)
				SELECT id, some_field AS processed_field FROM tmp
			`, ids).Scan(&resultData).Error
			if err != nil {
				log.Printf("Worker %d: Error processing ID group: %v", workerID, err)
				continue
			}

			// Step 3: Insert results into the target table
			for _, result := range resultData {
				err = db.Exec("INSERT INTO b (processed_data) VALUES (?)", result.ProcessedField).Error
				if err != nil {
					log.Printf("Worker %d: Error inserting data: %v", workerID, err)
				}
			}

			// Step 4: Mark IDs as processed only after processing and insertion
			func() {
				stackMutex.Lock()
				defer stackMutex.Unlock()

				for _, result := range resultData {
					processedStack[result.ID] = true
				}
			}()

			log.Printf("Worker %d: Processed IDs: %v", workerID, ids)
		}
	}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(i)
	}

	// Wait for workers to finish
	wg.Wait()
	log.Println("Processing complete")
}

// keys returns the keys of a map as a slice.
func keys(m map[uint]bool) []uint {
	ids := make([]uint, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	return ids
}
