package api

import (
	"regexp"
	"strconv"
	"strings"
)

// macPattern 对齐 xiaozhi-java CommonUtils.MAC_PATTERN:
// ^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$
var macPattern = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)

// IsMacAddressValid 对齐 xiaozhi-java CommonUtils.isMacAddressValid：
// 校验 MAC 地址格式正确且为单播地址（首字节最低位 = 0）。
//
// ESP32 设备会以十六进制字符串形式通过 "Device-Id" header 上报 MAC。
// 该函数同时接受冒号或短横作为分隔符（大小写不敏感）。
func IsMacAddressValid(mac string) bool {
	if !macPattern.MatchString(mac) {
		return false
	}
	// 提取首字节：取第一个分隔符前 2 个字符
	idx := strings.IndexAny(mac, ":-")
	// bitSize=16：覆盖 0x00-0xFF（8-bit 无符号），避开 ParseInt 默认有符号范围 [-128,127]
	firstByte, err := strconv.ParseInt(mac[:idx], 16, 16)
	if err != nil {
		return false
	}
	return (firstByte & 1) == 0
}