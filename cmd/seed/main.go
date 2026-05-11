package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	baseURL     = "https://api.cms.reearth.io"
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

// --- GeoJSON types ---

type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

type feature struct {
	Type       string            `json:"type"`
	Geometry   pointGeometry     `json:"geometry"`
	Properties siteProperties    `json:"properties"`
}

type pointGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"` // [lng, lat]
}

type siteProperties struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	City        string `json:"city"`
	Description string `json:"description"`
}

// --- Import API response ---

type importResponse struct {
	ModelID       string `json:"modelId"`
	ItemsCount    int    `json:"itemsCount"`
	InsertedCount int    `json:"insertedCount"`
	UpdatedCount  int    `json:"updatedCount"`
	IgnoredCount  int    `json:"ignoredCount"`
}

// --- Seed data ---

type site struct {
	Name        string
	Category    string
	Lng         float64
	Lat         float64
	City        string
	Description string
}

var sites = []site{
	{"Senso-ji Temple", "temple", 139.7967, 35.7148, "Tokyo", "Tokyo's oldest temple, founded in 628 AD"},
	{"Fushimi Inari Shrine", "shrine", 135.7727, 34.9671, "Kyoto", "Famous for thousands of torii gates"},
	{"Himeji Castle", "castle", 134.6939, 34.8394, "Himeji", "Best preserved castle in Japan, UNESCO site"},
	{"Kenroku-en Garden", "garden", 136.6621, 36.5613, "Kanazawa", "One of Japan's three great gardens"},
	{"Meiji Shrine", "shrine", 139.6993, 35.6763, "Tokyo", "Shinto shrine dedicated to Emperor Meiji"},
	{"Kinkaku-ji Temple", "temple", 135.7292, 35.0394, "Kyoto", "The Golden Pavilion, covered in gold leaf"},
	{"Matsumoto Castle", "castle", 137.9719, 36.2381, "Matsumoto", "One of Japan's premier historic castles"},
	{"Shinjuku Gyoen", "garden", 139.7100, 35.6852, "Tokyo", "Large national garden with French and English styles"},
	{"Itsukushima Shrine", "shrine", 132.3197, 34.2960, "Hiroshima", "Famous floating torii gate on the sea"},
	{"Todai-ji Temple", "temple", 135.8398, 34.6888, "Nara", "Houses Japan's largest bronze Buddha statue"},
}

// buildGeoJSON converts the seed data into a GeoJSON FeatureCollection.
func buildGeoJSON() ([]byte, error) {
	fc := featureCollection{
		Type:     "FeatureCollection",
		Features: make([]feature, len(sites)),
	}
	for i, s := range sites {
		fc.Features[i] = feature{
			Type: "Feature",
			Geometry: pointGeometry{
				Type:        "Point",
				Coordinates: []float64{s.Lng, s.Lat},
			},
			Properties: siteProperties{
				Name:        s.Name,
				Category:    s.Category,
				City:        s.City,
				Description: s.Description,
			},
		}
	}
	return json.MarshalIndent(fc, "", "  ")
}

// runImport sends the GeoJSON to the CMS Import API as multipart/form-data.
func runImport(token, workspace, project, model, strategy, geometryField string, geoJSON []byte) (*importResponse, error) {
	// Build multipart body
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", "landmarks.geojson")
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(geoJSON); err != nil {
		return nil, err
	}
	_ = mw.WriteField("format", "geoJson")
	_ = mw.WriteField("strategy", strategy)
	if err := mw.Close(); err != nil {
		return nil, err
	}

	// geometryFieldKey goes as a query param
	endpoint := fmt.Sprintf("%s/%s/projects/%s/models/%s/import?geometryFieldKey=%s",
		baseURL, workspace, project, model, url.QueryEscape(geometryField))

	req, err := http.NewRequest("PUT", endpoint, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result importResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func main() {
	dryRun        := flag.Bool("dry-run", false, "print GeoJSON to stdout instead of calling the API")
	strategy      := flag.String("strategy", "insert", "import strategy: insert | update | upsert")
	geometryField := flag.String("geometry-field", "location", "geometry field key in the CMS model")
	flag.Parse()

	// --dry-run: just emit the GeoJSON, no env vars needed
	if *dryRun {
		geoJSON, err := buildGeoJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		fmt.Println(string(geoJSON))
		return
	}

	loadEnv(".env")
	token     := mustEnv("CMS_TOKEN")
	workspace := mustEnv("CMS_WORKSPACE")
	project   := mustEnv("CMS_PROJECT")
	model     := mustEnv("CMS_MODEL")

	fmt.Printf("\n%s%s Re:Earth CMS Seeder%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s\n\n", strings.Repeat("─", 42))
	fmt.Printf("Workspace: %s\nProject:   %s\nModel:     %s\n", workspace, project, model)
	fmt.Printf("Strategy:  %s  |  Geometry field: %s\n\n", *strategy, *geometryField)

	fmt.Printf("Building GeoJSON (%d features)... ", len(sites))
	geoJSON, err := buildGeoJSON()
	if err != nil {
		fmt.Printf("%sFAILED%s: %s\n", colorRed, colorReset, err)
		os.Exit(1)
	}
	fmt.Printf("%sOK%s (%d bytes)\n", colorGreen, colorReset, len(geoJSON))

	fmt.Print("Uploading...            ")
	result, err := runImport(token, workspace, project, model, *strategy, *geometryField, geoJSON)
	if err != nil {
		fmt.Printf("%sFAILED%s\n  %s\n", colorRed, colorReset, err)
		os.Exit(1)
	}

	fmt.Printf("%sOK%s\n\n", colorGreen, colorReset)
	fmt.Printf("%s\n", strings.Repeat("─", 42))
	fmt.Printf("Model ID:  %s\n", result.ModelID)
	fmt.Printf("Total:     %d\n", result.ItemsCount)
	fmt.Printf("Inserted:  %s%d%s\n", colorGreen, result.InsertedCount, colorReset)
	if result.UpdatedCount > 0 {
		fmt.Printf("Updated:   %s%d%s\n", colorYellow, result.UpdatedCount, colorReset)
	}
	if result.IgnoredCount > 0 {
		fmt.Printf("Ignored:   %d\n", result.IgnoredCount)
	}
	fmt.Println()
}

func loadEnv(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		fmt.Fprintf(os.Stderr, "%sError:%s missing required env var: %s\n", colorRed, colorReset, key)
		os.Exit(1)
	}
	return val
}
