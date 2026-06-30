package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultVendorRulesUseEnglishNames(t *testing.T) {
	expected := map[string]string{
		"chatglm": "Zhipu",
		"glm-":    "Zhipu",
		"qwen":    "Alibaba",
		"ernie":   "Baidu",
		"spark":   "iFLYTEK",
		"hunyuan": "Tencent",
		"yi":      "01.AI",
		"doubao":  "ByteDance",
		"kling":   "Kuaishou",
		"jimeng":  "Jimeng",
	}

	for pattern, vendorName := range expected {
		assert.Equal(t, vendorName, defaultVendorRules[pattern])
	}
}

func TestDefaultVendorIconsSupportEnglishAndLegacyChineseNames(t *testing.T) {
	cases := map[string]string{
		"Zhipu":     "Zhipu.Color",
		"智谱":        "Zhipu.Color",
		"Alibaba":   "Qwen.Color",
		"阿里巴巴":      "Qwen.Color",
		"Baidu":     "Wenxin.Color",
		"百度":        "Wenxin.Color",
		"iFLYTEK":   "Spark.Color",
		"讯飞":        "Spark.Color",
		"Tencent":   "Hunyuan.Color",
		"腾讯":        "Hunyuan.Color",
		"01.AI":     "Yi.Color",
		"零一万物":      "Yi.Color",
		"ByteDance": "Doubao.Color",
		"字节跳动":      "Doubao.Color",
		"Kuaishou":  "Kling.Color",
		"快手":        "Kling.Color",
		"Jimeng":    "Jimeng.Color",
		"即梦":        "Jimeng.Color",
	}

	for vendorName, icon := range cases {
		assert.Equal(t, icon, getDefaultVendorIcon(vendorName))
	}
}

func TestDefaultVendorDisplayNameTranslatesLegacyChineseNames(t *testing.T) {
	cases := map[string]string{
		"智谱":       "Zhipu",
		"阿里巴巴":     "Alibaba",
		"百度":       "Baidu",
		"讯飞":       "iFLYTEK",
		"腾讯":       "Tencent",
		"零一万物":     "01.AI",
		"字节跳动":     "ByteDance",
		"快手":       "Kuaishou",
		"即梦":       "Jimeng",
		"DeepSeek": "DeepSeek",
	}

	for vendorName, displayName := range cases {
		assert.Equal(t, displayName, getDefaultVendorDisplayName(vendorName))
	}
}
