package render

import "testing"

func TestLocalizeWeatherCondition(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		fallback string
		locale   string
		want     string
	}{
		{name: "clear in Korean", code: 800, fallback: "clear sky", locale: "ko", want: "맑음"},
		{name: "clouds in Korean", code: 801, fallback: "few clouds", locale: "ko", want: "구름 조금"},
		{name: "rain in Korean", code: 501, fallback: "moderate rain", locale: "ko", want: "비"},
		{name: "snow showers in Korean", code: 621, fallback: "shower snow", locale: "ko", want: "눈 소나기"},
		{name: "English keeps provider description", code: 800, fallback: "clear sky", locale: "en", want: "clear sky"},
		{name: "unknown code falls back", code: 999, fallback: "unrecognized condition", locale: "ko", want: "unrecognized condition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localizeWeatherCondition(tt.code, tt.fallback, tt.locale); got != tt.want {
				t.Errorf("localizeWeatherCondition(%d, %q, %q) = %q, want %q", tt.code, tt.fallback, tt.locale, got, tt.want)
			}
		})
	}
}
