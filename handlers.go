package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var templateFuncs = template.FuncMap{
	"subtract": func(a, b int) int { return a - b },
	"add":      func(a, b int) int { return a + b },
}

// Modify the handleHome function in handlers.go
func handleHome(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	region := r.URL.Query().Get("region")
	timezone := r.URL.Query().Get("timezone")
	timeRange := r.URL.Query().Get("timerange")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	filteredCountries := filterCountries(allCountries, region, timezone, timeRange, w, r)
	if filteredCountries == nil {
		return
	}

	searchedCountries := searchCountries(filteredCountries, query)

	// Check if search query was provided and no results were found
	if query != "" && len(searchedCountries) == 0 {
		http.Redirect(w, r, "/error?type=search&query="+query, http.StatusSeeOther)
		return
	}

	// Calculate total pages before checking page bounds
	_, totalPages := paginateCountries(searchedCountries, 1)

	// Check if page is greater than the total number of pages
	if page > totalPages && totalPages > 0 {
		http.Redirect(w, r, "/error?type=page&max="+strconv.Itoa(totalPages), http.StatusSeeOther)
		return
	}

	paginatedCountries, _ := paginateCountries(searchedCountries, page)
	regions := getUniqueRegions(allCountries)
	timeZones := getUniqueTimeZones(allCountries)

	data := PageData{
		Countries:    paginatedCountries,
		Query:        query,
		Regions:      regions,
		TimeZones:    timeZones,
		CurrentPage:  page,
		TotalPages:   totalPages,
		Region:       region,
		TimeZone:     timezone,
		TimeRange:    timeRange,
		ItemsPerPage: itemsPerPage,
	}

	tmpl := template.New("home.html").Funcs(templateFuncs)
	tmpl, err := tmpl.ParseFiles("templates/home.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func handleFavorites(w http.ResponseWriter, r *http.Request) {
	var favoriteCountries []Country
	for _, country := range allCountries {
		if contains(favorites.Countries, country.Name) {
			country.IsFavorite = true
			favoriteCountries = append(favoriteCountries, country)
		}
	}

	data := PageData{
		Countries: favoriteCountries,
	}

	tmpl, err := template.ParseFiles("templates/favorites.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func handleFavoriteAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		Country string `json:"country"`
		Action  string `json:"action"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	switch data.Action {
	case "add":
		if !contains(favorites.Countries, data.Country) {
			favorites.Countries = append(favorites.Countries, data.Country)
		}
	case "remove":
		var newFavorites []string
		for _, c := range favorites.Countries {
			if c != data.Country {
				newFavorites = append(newFavorites, c)
			}
		}
		favorites.Countries = newFavorites
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if err := saveFavorites(); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleAbout(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/about.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, nil)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func handleError(w http.ResponseWriter, r *http.Request) {
	errorType := r.URL.Query().Get("type")
	query := r.URL.Query().Get("query")
	maxPage := r.URL.Query().Get("max")

	errorData := struct {
		ErrorTitle   string
		ErrorMessage string
		Suggestions  []string
	}{}

	// Default error content
	errorData.ErrorTitle = "Error"
	errorData.ErrorMessage = "An unexpected error occurred."
	errorData.Suggestions = []string{"Return to the homepage", "Try again later"}

	// Handle specific error types
	switch errorType {
	case "search":
		errorData.ErrorTitle = "No Results Found"
		errorData.ErrorMessage = "No countries match your search criteria: '" + query + "'"
		errorData.Suggestions = []string{
			"Check your spelling",
			"Try a more general search term",
			"Search by region instead",
			"Browse all countries without filters",
		}
	case "timezone":
		errorData.ErrorTitle = "No Countries in Time Zone"
		errorData.ErrorMessage = "We couldn't find any countries in the selected time zone."
		errorData.Suggestions = []string{
			"Try a different time zone",
			"Check our world map to see time zone coverage",
			"Browse all countries without time zone filter",
		}
	case "page":
		errorData.ErrorTitle = "Invalid Page Number"
		errorData.ErrorMessage = "The requested page number does not exist."
		if maxPage != "" {
			if maxPage == "1" {
				errorData.ErrorMessage += " There is only 1 page available."
			} else {
				errorData.ErrorMessage += " Available pages: 1 to " + maxPage + "."
			}
		}
		errorData.Suggestions = []string{
			"Go to the first page",
			"Use the pagination controls at the bottom of the page",
			"Return to the homepage without filters",
		}
	}

	tmpl, err := template.ParseFiles("templates/error.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	err = tmpl.Execute(w, errorData)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func handleMap(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/map.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, nil)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func handleCountriesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allCountries)
}

func handleTimezoneBorders(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request for timezone borders")

	// Define the directory where split GeoJSON files are stored
	dataDir := "data"
	files, err := os.ReadDir(dataDir)
	if err != nil {
		log.Printf("Error reading data directory: %v", err)
		http.Error(w, "Timezone data not available", http.StatusInternalServerError)
		return
	}

	// Collect all GeoJSON file contents
	var allFeatures []map[string]interface{}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".geojson") {
			filePath := filepath.Join(dataDir, file.Name())
			log.Printf("Processing file: %s", filePath)

			// Open and parse each GeoJSON file
			content, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Error reading file %s: %v", filePath, err)
				continue
			}

			var geojson map[string]interface{}
			if err := json.Unmarshal(content, &geojson); err != nil {
				log.Printf("Error parsing GeoJSON from file %s: %v", filePath, err)
				continue
			}

			// Append features from this file
			if features, ok := geojson["features"].([]interface{}); ok {
				for _, feature := range features {
					allFeatures = append(allFeatures, feature.(map[string]interface{}))
				}
			}
		}
	}

	// Prepare combined GeoJSON
	combinedGeoJSON := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": allFeatures,
	}

	// Set headers and send response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	if err := json.NewEncoder(w).Encode(combinedGeoJSON); err != nil {
		log.Printf("Error encoding combined GeoJSON: %v", err)
		http.Error(w, "Error processing GeoJSON", http.StatusInternalServerError)
	}
}
