package render

type weatherConditionTranslation struct {
	first int
	last  int
	text  string
}

// Keep translations grouped by locale so another language can be added as a
// new entry without changing the lookup logic below.
var weatherConditionTranslations = map[string][]weatherConditionTranslation{
	"ko": {
		{200, 232, "천둥번개"},
		{300, 321, "이슬비"},
		{500, 504, "비"},
		{511, 511, "어는 비"},
		{520, 531, "소나기"},
		{600, 602, "눈"},
		{611, 616, "진눈깨비"},
		{620, 622, "눈 소나기"},
		{701, 701, "박무"},
		{711, 711, "연기"},
		{721, 721, "연무"},
		{731, 731, "모래·먼지 회오리"},
		{741, 741, "안개"},
		{751, 751, "모래"},
		{761, 761, "먼지"},
		{762, 762, "화산재"},
		{771, 771, "돌풍"},
		{781, 781, "토네이도"},
		{800, 800, "맑음"},
		{801, 801, "구름 조금"},
		{802, 802, "구름 있음"},
		{803, 803, "구름 많음"},
		{804, 804, "흐림"},
	},
}

func localizeWeatherCondition(code int, fallback, locale string) string {
	for _, translation := range weatherConditionTranslations[locale] {
		if code >= translation.first && code <= translation.last {
			return translation.text
		}
	}
	return fallback
}
