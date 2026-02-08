package models

import (
	"encoding/json"
	"fmt"
)

// FlexibleString can unmarshal both strings and numbers from JSON
type FlexibleString string

func (fs *FlexibleString) UnmarshalJSON(data []byte) error {
	// Handle null values
	if string(data) == "null" {
		*fs = FlexibleString("")
		return nil
	}
	
	// Try to unmarshal as string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*fs = FlexibleString(s)
		return nil
	}
	
	// If that fails, try as number and convert to string
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		*fs = FlexibleString(fmt.Sprintf("%.0f", n))
		return nil
	}
	
	// Try as integer as well
	var i int64
	if err := json.Unmarshal(data, &i); err == nil {
		*fs = FlexibleString(fmt.Sprintf("%d", i))
		return nil
	}
	
	return fmt.Errorf("cannot unmarshal %s into FlexibleString", string(data))
}

// String returns the string value
func (fs FlexibleString) String() string {
	return string(fs)
}

type Dump1090fa struct {
	Now      float64 `json:"now"`
	Messages int     `json:"messages"`
	Aircraft []struct {
		Hex            string        `json:"hex"`
		Type           string        `json:"type"`
		Flight         string        `json:"flight,omitempty"`
		R              string        `json:"r"`
		T              string        `json:"t"`
		Desc           string        `json:"desc"`
		AltBaro        FlexibleString `json:"alt_baro,omitempty"`
		AltGeom        int           `json:"alt_geom,omitempty"`
		Gs             float64       `json:"gs,omitempty"`
		Ias            int           `json:"ias,omitempty"`
		Tas            int           `json:"tas,omitempty"`
		Mach           float64       `json:"mach,omitempty"`
		Wd             int           `json:"wd,omitempty"`
		Ws             int           `json:"ws,omitempty"`
		Oat            int           `json:"oat,omitempty"`
		Tat            int           `json:"tat,omitempty"`
		Track          float64       `json:"track,omitempty"`
		TrackRate      float64       `json:"track_rate,omitempty"`
		Roll           float64       `json:"roll,omitempty"`
		MagHeading     float64       `json:"mag_heading,omitempty"`
		TrueHeading    float64       `json:"true_heading,omitempty"`
		BaroRate       int           `json:"baro_rate,omitempty"`
		GeomRate       int           `json:"geom_rate,omitempty"`
		Squawk         string        `json:"squawk,omitempty"`
		Category       string        `json:"category,omitempty"`
		NavQnh         float64       `json:"nav_qnh,omitempty"`
		NavAltitudeMcp int           `json:"nav_altitude_mcp,omitempty"`
		NavHeading     float64       `json:"nav_heading,omitempty"`
		Lat            float64       `json:"lat,omitempty"`
		Lon            float64       `json:"lon,omitempty"`
		Nic            int           `json:"nic,omitempty"`
		Rc             int           `json:"rc,omitempty"`
		SeenPos        float64       `json:"seen_pos,omitempty"`
		RDst           float64       `json:"r_dst,omitempty"`
		RDir           float64       `json:"r_dir,omitempty"`
		Version        int           `json:"version,omitempty"`
		NicBaro        int           `json:"nic_baro,omitempty"`
		NacP           int           `json:"nac_p,omitempty"`
		NacV           int           `json:"nac_v,omitempty"`
		Sil            int           `json:"sil,omitempty"`
		SilType        string        `json:"sil_type"`
		Gva            int           `json:"gva,omitempty"`
		Sda            int           `json:"sda,omitempty"`
		Alert          int           `json:"alert,omitempty"`
		Spi            int           `json:"spi,omitempty"`
		Mlat           []interface{} `json:"mlat"`
		Tisb           []interface{} `json:"tisb"`
		Messages       int           `json:"messages"`
		Seen           float64       `json:"seen"`
		Rssi           float64       `json:"rssi"`
		NavAltitudeFms int           `json:"nav_altitude_fms,omitempty"`
		OwnOp          string        `json:"ownOp,omitempty"`
		Year           string        `json:"year,omitempty"`
		Emergency      string        `json:"emergency,omitempty"`
		NavModes       []string      `json:"nav_modes,omitempty"`
		DbFlags        int           `json:"dbFlags,omitempty"`
		LastPosition   struct {
			Lat     float64 `json:"lat"`
			Lon     float64 `json:"lon"`
			Nic     int     `json:"nic"`
			Rc      int     `json:"rc"`
			SeenPos float64 `json:"seen_pos"`
		} `json:"lastPosition,omitempty"`
	} `json:"aircraft"`
}
