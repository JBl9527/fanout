package main

import (
	"testing"
)

func TestJudgeIPTypeUSResidential(t *testing.T) {
	cases := []struct {
		info     IPInfo
		wantRes  bool
		wantType string
	}{
		{
			info: IPInfo{
				CountryCode: "US",
				ISP:         "Comcast Cable Communications, Inc.",
				Hosting:     false,
				Proxy:       false,
			},
			wantRes:  true,
			wantType: "美国家宽",
		},
		{
			info: IPInfo{
				CountryCode: "US",
				ISP:         "Charter Communications Inc",
				Org:         "Spectrum",
				Hosting:     false,
			},
			wantRes:  true,
			wantType: "美国家宽",
		},
		{
			info: IPInfo{
				CountryCode: "US",
				ISP:         "DigitalOcean, LLC",
				Hosting:     true,
			},
			wantRes:  false,
			wantType: "机房 IP",
		},
		{
			info: IPInfo{
				CountryCode: "US",
				ISP:         "Amazon.com, Inc.",
				Org:         "AWS",
				Hosting:     false,
			},
			wantRes:  false,
			wantType: "机房 IP",
		},
		{
			info: IPInfo{
				CountryCode: "JP",
				ISP:         "NTT Communications",
				Hosting:     false,
			},
			wantRes:  false,
			wantType: "住宅宽带",
		},
	}

	for i, tc := range cases {
		item := tc.info
		JudgeIPType(&item)
		if item.IsResidential != tc.wantRes {
			t.Errorf("case %d: IsResidential got %v, want %v", i, item.IsResidential, tc.wantRes)
		}
		if item.IPType != tc.wantType {
			t.Errorf("case %d: IPType got %q, want %q", i, item.IPType, tc.wantType)
		}
	}
}

func TestIsUSResidentialHost(t *testing.T) {
	if !isUSResidentialHost("c-73-189-10-20.hsd1.ca.comcast.net") {
		t.Error("comcast host should be detected as residential")
	}
	if !isUSResidentialHost("12-34-56-78.res.rr.com") {
		t.Error("rr.com host should be detected as residential")
	}
	if isUSResidentialHost("vps123.digitalocean.com") {
		t.Error("digitalocean host should not be detected as US residential")
	}
}
