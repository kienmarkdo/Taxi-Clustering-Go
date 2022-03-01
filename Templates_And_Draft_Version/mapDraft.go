// Project CSI2120/CSI2520
// Winter 2022
// Robert Laganiere, uottawa.ca

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"time"
)

type GPScoord struct {
	lat  float64
	long float64
}

type LabelledGPScoord struct {
	GPScoord
	ID    int // point ID
	Label int // cluster ID
}

type Job struct {
	coords []LabelledGPScoord
	minPts int
	eps    float64
	offset int
}

// Utilisation d'une semaphore pour la synchronisation
type semaphore chan bool

// fonction qui va attendre toutes les go routines
func (s semaphore) Wait(n int) {
	for i := 0; i < n; i++ {
		<-s
	}
}

// fonction qui signale la fin d'une go routine
func (s semaphore) Signal() {
	s <- true
}

const ConsumerCount = 4
const N int = 4
const MinPts int = 5
const eps float64 = 0.0003
const filename string = "yellow_tripdata_2009-01-15_9h_21h_clean.csv"

func main() {

	start := time.Now()

	gps, minPt, maxPt := readCSVFile(filename)
	fmt.Printf("Number of points: %d\n", len(gps))

	minPt = GPScoord{-74., 40.7}
	maxPt = GPScoord{-73.93, 40.8}

	// geographical limits
	fmt.Printf("NW:(%f , %f)\n", minPt.long, minPt.lat)
	fmt.Printf("SE:(%f , %f) \n\n", maxPt.long, maxPt.lat)

	// Parallel DBSCAN STEP 1.
	incx := (maxPt.long - minPt.long) / float64(N)
	incy := (maxPt.lat - minPt.lat) / float64(N)

	var grid [N][N][]LabelledGPScoord // a grid of GPScoord slices

	// Create the partition
	// triple loop! not very efficient, but easier to understand

	partitionSize := 0
	for j := 0; j < N; j++ {
		for i := 0; i < N; i++ {

			for _, pt := range gps {

				// is it inside the expanded grid cell
				if (pt.long >= minPt.long+float64(i)*incx-eps) && (pt.long < minPt.long+float64(i+1)*incx+eps) && (pt.lat >= minPt.lat+float64(j)*incy-eps) && (pt.lat < minPt.lat+float64(j+1)*incy+eps) {

					grid[i][j] = append(grid[i][j], pt) // add the point to this slide
					partitionSize++
				}
			}
		}
	}

	// ***
	// This is the non-concurrent procedural version
	// It should be replaced by a producer thread that produces jobs (partition to be clustered)
	// And by consumer threads that clusters partitions
	// for j := 0; j < N; j++ {
	// 	for i := 0; i < N; i++ {

	// 		DBscan(grid[i][j], MinPts, eps, i*10000000+j*1000000)
	// 	}
	// }

	// Parallel DBSCAN STEP 2.
	// Apply DBSCAN on each partition
	// ...
	// // i := 0
	// // j := 0
	// // DBscan(grid[i][j], MinPts, eps, i*10000000+j*1000000)

	// i := 0
	// j := 2
	// DBscan(grid[i][j], MinPts, eps, i*10000000+j*1000000)

	// i = 0
	// j = 3
	// DBscan(grid[i][j], MinPts, eps, i*10000000+j*1000000)

	jobs := make(chan Job)
	mutex := make(semaphore)

	fmt.Printf("N = %d and %d consumer threads.\n\n", N, ConsumerCount)

	for i := 0; i < ConsumerCount; i++ {
		go consume(jobs, mutex)
	}

	for j := 0; j < N; j++ {
		for i := 0; i < N; i++ {
			jobs <- Job{grid[i][j], MinPts, eps, i*10000000 + j*1000000}
		}
	}

	close(jobs)
	mutex.Wait(ConsumerCount)

	// Parallel DBSCAN step 3.
	// merge clusters
	// *DO NOT PROGRAM THIS STEP

	end := time.Now()
	fmt.Printf("\nExecution time: %s of %d points\n", end.Sub(start), partitionSize)
}

func consume(jobs <-chan Job, sem semaphore) {

	for {
		j, more := <-jobs

		if more {
			DBscan(&j.coords, j.minPts, j.eps, j.offset)
		} else {
			sem.Signal()
			return
		}
	}
}

// Applies DBSCAN algorithm on LabelledGPScoord points
// LabelledGPScoord: the slice of LabelledGPScoord points
// MinPts, eps: parameters for the DBSCAN algorithm
// offset: label of first cluster (also used to identify the cluster)
// returns number of clusters found
func DBscan(coords *[]LabelledGPScoord, MinPts int, eps float64, offset int) (nclusters int) {

	nclusters = 0

	for p := 0; p < len(*coords); p++ {
		if (*coords)[p].Label != 0 { // undefined
			continue
		}

		neighbours := rangeQuery(*coords, (*coords)[p], eps)

		if len(neighbours) < MinPts {
			(*coords)[p].Label = -1 // noise
			continue
		}

		nclusters++
		(*coords)[p].Label = nclusters + offset

		var seedSet []*LabelledGPScoord
		seedSet = append(seedSet, neighbours...)

		for q := 0; q < len(seedSet); q++ {
			if seedSet[q].Label == -1 { // noise
				seedSet[q].Label = nclusters + offset
			}

			if seedSet[q].Label != 0 { // undefined
				continue
			}

			seedSet[q].Label = nclusters + offset

			seedNeighbours := rangeQuery(*coords, *seedSet[q], eps)
			if len(seedNeighbours) >= MinPts {
				// addNeighbours(&seedSet, seedNeighbours)
				seedSet = append(seedSet, seedNeighbours...)
				//seedSet = removeDuplicateGPS(seedSet)

			}
		} // end of inner for loop

	} // end of outer for loop

	// Printing the result (do not remove)
	fmt.Printf("Partition %10d : [%4d,%6d]\n", offset, nclusters, len(*coords))

	return nclusters
}

func rangeQuery(db []LabelledGPScoord, p LabelledGPScoord, eps float64) []*LabelledGPScoord {

	var neighbours []*LabelledGPScoord

	for i := 0; i < len(db); i++ {
		if calculateDistance(p, db[i]) <= eps {
			neighbours = append(neighbours, &db[i])
		}
	}
	return neighbours
}

func calculateDistance(p1 LabelledGPScoord, p2 LabelledGPScoord) float64 {
	return math.Sqrt((p1.lat-p2.lat)*(p1.lat-p2.lat) + (p1.long-p2.long)*(p1.long-p2.long))
}

// func addNeighbours(seed *[]LabelledGPScoord, neighbours []LabelledGPScoord) {
// 	for i := range neighbours {
// 		if !contains(*seed, neighbours[i]) {
// 			*seed = append(*seed, neighbours[i])
// 		}
// 	}
// }

// func contains(seed []LabelledGPScoord, point LabelledGPScoord) bool {
// 	for i := range seed {
// 		if seed[i] == point {
// 			return true
// 		}
// 	}
// 	return false
// }

func removeDuplicateGPS(gpsSlice []*LabelledGPScoord) []*LabelledGPScoord {
	allKeys := make(map[LabelledGPScoord]bool)
	list := []*LabelledGPScoord{}
	for _, item := range gpsSlice {
		if _, value := allKeys[*item]; !value {
			allKeys[*item] = true
			list = append(list, &(*item))
		}
	}
	return list
}

// reads a csv file of trip records and returns a slice of the LabelledGPScoord of the pickup locations
// and the minimum and maximum GPS coordinates
func readCSVFile(filename string) (coords []LabelledGPScoord, minPt GPScoord, maxPt GPScoord) {

	coords = make([]LabelledGPScoord, 0, 5000)

	// open csv file
	src, err := os.Open(filename)
	defer src.Close()
	if err != nil {
		panic("File not found...")
	}

	// read and skip first line
	r := csv.NewReader(src)
	record, err := r.Read()
	if err != nil {
		panic("Empty file...")
	}

	minPt.long = 1000000.
	minPt.lat = 1000000.
	maxPt.long = -1000000.
	maxPt.lat = -1000000.

	var n int = 0

	for {
		// read line
		record, err = r.Read()

		// end of file?
		if err == io.EOF {
			break
		}

		if err != nil {
			panic("Invalid file format...")
		}

		// get lattitude
		lat, err := strconv.ParseFloat(record[8], 64)
		if err != nil {
			fmt.Printf("\n%d lat=%s\n", n, record[8])
			panic("Data format error (lat)...")
		}

		// is corner point?
		if lat > maxPt.lat {
			maxPt.lat = lat
		}
		if lat < minPt.lat {
			minPt.lat = lat
		}

		// get longitude
		long, err := strconv.ParseFloat(record[9], 64)
		if err != nil {
			panic("Data format error (long)...")
		}

		// is corner point?
		if long > maxPt.long {
			maxPt.long = long
		}

		if long < minPt.long {
			minPt.long = long
		}

		// add point to the slice
		n++
		pt := GPScoord{lat, long}
		coords = append(coords, LabelledGPScoord{pt, n, 0})
	}

	return coords, minPt, maxPt
}
