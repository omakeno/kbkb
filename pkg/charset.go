package kbkb

func GetKbkbCharSet() KbkbCharSet {
	return KbkbCharSet{
		ColorCodeSet: map[string]string{
			"red":    "31m",
			"green":  "32m",
			"yellow": "33m",
			"blue":   "34m",
			"purple": "35m",
		},
		Wall:         "|",
		Floor:        "-",
		LeftCorner:   "+",
		RightCorner:  "+",
		StableIcon:   "@",
		UnstableIcon: "o",
		Blank:        " ",
	}
}

func GetKbkbCharSetWide() KbkbCharSet {
	return KbkbCharSet{
		ColorCodeSet: map[string]string{
			"red":    "31m",
			"green":  "32m",
			"yellow": "33m",
			"blue":   "34m",
			"purple": "35m",
		},
		Wall:         "|",
		Floor:        "--",
		LeftCorner:   "+",
		RightCorner:  "+",
		StableIcon:   "●",
		UnstableIcon: "○",
		Blank:        "  ",
	}
}
