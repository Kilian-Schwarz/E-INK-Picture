package services

type WeatherService struct {
	apiKey   string
	location string
}

func NewWeatherService(apiKey, location string) *WeatherService {
	return &WeatherService{apiKey: apiKey, location: location}
}

func (s *WeatherService) Fetch() (any, error) {
	return nil, nil
}
