package nocpass

import (
	"errors"
	"strings"

	"github.com/Flafl/DevOpsCore/internal/models"
)

const (
	TargetDevice      = "device"
	TargetAllNetworks = "all_networks"
	TargetNetworkType = "network_type"
	TargetProvince    = "province"
	TargetVendor      = "vendor"
	TargetModel       = "model"
)

func NormalizeSourceVendor(vendor, model string) string {
	normalizedVendor := strings.ToLower(strings.TrimSpace(vendor))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if (normalizedVendor == "cisco_ios" || normalizedVendor == "cisco_nexus" || normalizedVendor == "cisco") &&
		(strings.Contains(normalizedModel, "nexus") || strings.Contains(normalizedModel, "n9k") || strings.Contains(normalizedModel, "nexus9000")) {
		return "cisco_nexus"
	}
	return normalizedVendor
}

func ProvinceFromSite(site string) string {
	trimmed := strings.TrimSpace(site)
	if trimmed == "" {
		return "Unknown"
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return "Unknown"
	}
	first := strings.ToLower(parts[0])
	return strings.ToUpper(first[:1]) + first[1:]
}

func NetworkTypeFromSite(site string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(site)), "ftth") {
		return "ftth"
	}
	return "wifi"
}

func VendorDisplayLabel(vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_nexus":
		return "Cisco Nexus"
	case "cisco_ios", "cisco":
		return "Cisco"
	case "mikrotik":
		return "Microtik"
	case "huawei":
		return "Huawei"
	case "nokia":
		return "Nokia"
	default:
		if vendor == "" {
			return "Unknown"
		}
		return vendor
	}
}

func DefaultTargetLabel(targetType, targetValue string, row *models.NocDataDevice) string {
	switch targetType {
	case TargetDevice:
		if row != nil {
			if strings.TrimSpace(row.Hostname) != "" {
				return strings.TrimSpace(row.Hostname)
			}
			if strings.TrimSpace(row.DisplayName) != "" {
				return strings.TrimSpace(row.DisplayName)
			}
			if strings.TrimSpace(row.Host) != "" {
				return strings.TrimSpace(row.Host)
			}
		}
		return "Device"
	case TargetAllNetworks:
		return "All Networks"
	case TargetNetworkType:
		if strings.EqualFold(strings.TrimSpace(targetValue), "ftth") {
			return "FTTH"
		}
		return "WiFi"
	case TargetProvince:
		return ProvinceFromSite(targetValue)
	case TargetVendor:
		return VendorDisplayLabel(targetValue)
	case TargetModel:
		return strings.TrimSpace(targetValue)
	default:
		return strings.TrimSpace(targetValue)
	}
}

func NormalizeGroupTarget(targetType, targetValue string) (string, string, error) {
	normalizedType := strings.ToLower(strings.TrimSpace(targetType))
	normalizedValue := strings.TrimSpace(targetValue)
	switch normalizedType {
	case TargetAllNetworks:
		return normalizedType, "all", nil
	case TargetNetworkType:
		value := strings.ToLower(normalizedValue)
		if value != "ftth" && value != "wifi" {
			return "", "", errors.New("invalid network type")
		}
		return normalizedType, value, nil
	case TargetProvince:
		if normalizedValue == "" {
			return "", "", errors.New("invalid province")
		}
		return normalizedType, ProvinceFromSite(normalizedValue), nil
	case TargetVendor:
		value := NormalizeSourceVendor(normalizedValue, "")
		switch value {
		case "cisco_ios", "cisco", "cisco_nexus", "mikrotik", "huawei", "nokia":
			return normalizedType, value, nil
		default:
			return "", "", errors.New("invalid vendor")
		}
	case TargetModel:
		if normalizedValue == "" {
			return "", "", errors.New("invalid model")
		}
		return normalizedType, normalizedValue, nil
	default:
		return "", "", errors.New("invalid target type")
	}
}

func TargetMatches(targetType, targetValue string, row *models.NocDataDevice) bool {
	switch strings.ToLower(strings.TrimSpace(targetType)) {
	case TargetAllNetworks:
		return true
	case TargetNetworkType:
		return NetworkTypeFromSite(row.Site) == strings.ToLower(strings.TrimSpace(targetValue))
	case TargetProvince:
		return strings.EqualFold(ProvinceFromSite(row.Site), targetValue)
	case TargetVendor:
		return strings.EqualFold(NormalizeSourceVendor(row.Vendor, row.DeviceModel), targetValue)
	case TargetModel:
		return strings.EqualFold(strings.TrimSpace(row.DeviceModel), targetValue)
	default:
		return false
	}
}
