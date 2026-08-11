package gpp

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	hexColorPattern = regexp.MustCompile(`^(?:#|0x)?(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	numberPattern   = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)
)

func validateFormatRule(value reflect.Value, rule validationRule) (bool, error) {
	text, err := validationString(value, rule.name)
	if err != nil {
		return false, err
	}
	switch rule.name {
	case "email":
		if text == "" {
			return true, nil
		}
		return isValidEmail(text), nil
	case "url", "http_url":
		return isValidURL(text, rule.name == "http_url"), nil
	case "ip", "ipv4", "ipv6":
		return isValidIP(text, rule.name), nil
	case "cidr", "cidrv4", "cidrv6":
		return isValidCIDR(text, rule.name), nil
	case "hostname":
		return isValidHostname(text), nil
	case "hostname_port":
		return isValidHostnamePort(text), nil
	case "uuid", "uuid3", "uuid4", "uuid5":
		return isValidUUID(text, rule.name), nil
	case "json":
		return json.Valid([]byte(text)), nil
	case "base64":
		return decodesBase64(text, base64.StdEncoding) || decodesBase64(text, base64.RawStdEncoding), nil
	case "base64url":
		return decodesBase64(text, base64.URLEncoding) || decodesBase64(text, base64.RawURLEncoding), nil
	case "hexadecimal":
		_, err := hex.DecodeString(text)
		return text != "" && len(text)%2 == 0 && err == nil, nil
	case "hexcolor":
		return hexColorPattern.MatchString(text), nil
	case "datetime":
		if rule.param == "" {
			return false, &validationConfigError{message: "datetime requires a layout"}
		}
		_, err := time.Parse(rule.param, text)
		return err == nil, nil
	default:
		return false, &validationConfigError{message: "invalid format rule"}
	}
}

func isValidEmail(value string) bool {
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && strings.Contains(address.Address, "@")
}

func isValidURL(value string, httpOnly bool) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if httpOnly {
		return parsed.Scheme == "http" || parsed.Scheme == "https"
	}
	return true
}

func isValidIP(value, rule string) bool {
	parsed := net.ParseIP(value)
	if parsed == nil {
		return false
	}
	switch rule {
	case "ipv4":
		return parsed.To4() != nil
	case "ipv6":
		return parsed.To4() == nil
	default:
		return true
	}
}

func isValidCIDR(value, rule string) bool {
	ip, _, err := net.ParseCIDR(value)
	if err != nil {
		return false
	}
	switch rule {
	case "cidrv4":
		return ip.To4() != nil
	case "cidrv6":
		return ip.To4() == nil
	default:
		return true
	}
}

func isValidHostnamePort(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil || host == "" || port == "" {
		return false
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return false
	}
	return net.ParseIP(host) != nil || isValidHostname(host)
}

func isValidHostname(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	value = strings.TrimSuffix(value, ".")
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func isValidUUID(value, rule string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != 16 {
		return false
	}
	if decoded[8]&0xc0 != 0x80 {
		return false
	}
	if rule == "uuid" {
		return true
	}
	wantedVersion := byte(rule[len(rule)-1] - '0')
	return decoded[6]>>4 == wantedVersion
}

func decodesBase64(value string, encoding *base64.Encoding) bool {
	if value == "" {
		return false
	}
	_, err := encoding.DecodeString(value)
	return err == nil
}
