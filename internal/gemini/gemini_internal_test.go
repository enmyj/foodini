package gemini

import (
	"strings"
	"testing"
)

func TestDayInsightsPrompt_NormalizesTreatsAndAvoidsCleverTone(t *testing.T) {
	required := []string{
		"Make it plain and useful",
		"Use plain language with a human voice",
		"Treats are part of normal eating",
		"not something to flag, fix, or compensate for",
		"A single treat, dessert, beer, or fast-food item is not an \"obviously lopsided\" day by itself",
		"Avoid implying guilt, damage, or a need to \"make up for\" a treat at the next meal",
		"Discuss blood sugar only when glucose data or a relevant medical context is provided",
	}
	for _, phrase := range required {
		if !strings.Contains(dayInsightsSystemPrompt, phrase) {
			t.Fatalf("dayInsightsSystemPrompt missing phrase %q", phrase)
		}
	}

	banned := []string{
		"sharp and a little fun",
		"light wit",
	}
	for _, phrase := range banned {
		if strings.Contains(dayInsightsSystemPrompt, phrase) {
			t.Fatalf("dayInsightsSystemPrompt still contains phrase %q", phrase)
		}
	}
}
