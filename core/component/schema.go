package component

import "fmt"

// FieldType 配置字段的类型，决定前端渲染成哪种控件。
type FieldType string

const (
	FieldText     FieldType = "text"     // 单行文本
	FieldPassword FieldType = "password" // 密码：界面打码、日志与导出中一律脱敏
	FieldNumber   FieldType = "number"   // 数字
	FieldBool     FieldType = "bool"     // 开关
	FieldSelect   FieldType = "select"   // 下拉，需要 Options
	FieldTextarea FieldType = "textarea" // 多行文本（例如证书 PEM）
	FieldFile     FieldType = "file"     // 文件上传，值是 base64 或已落盘的路径
	FieldDuration FieldType = "duration" // 时长，值是 Go duration 字符串，如 "24h"
)

// Option 下拉选项。
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field 一个配置字段的声明。
//
// 这是「声明式凭据 / 配置」的核心：组件只描述字段长什么样，
// 前端 SchemaForm 组件据此自动生成表单，后端也据此做校验与脱敏。
type Field struct {
	// Key 字段名，对应配置里的 JSON key。
	Key string `json:"key"`
	// Label 界面上显示的标签，中文。
	Label string `json:"label"`
	// Type 控件类型，留空视为 FieldText。
	Type FieldType `json:"type,omitempty"`
	// Required 是否必填。校验在提交时做。
	Required bool `json:"required,omitempty"`
	// Default 默认值。
	Default any `json:"default,omitempty"`
	// Placeholder 输入框占位提示。
	Placeholder string `json:"placeholder,omitempty"`
	// Help 字段下方的说明。写「去哪拿这个值」比写「这是什么」有用得多。
	Help string `json:"help,omitempty"`
	// Options Type 为 FieldSelect 时的可选项。
	Options []Option `json:"options,omitempty"`

	// Secret 标记敏感字段。影响三件事：
	//   1. 界面回显时打码（只显示是否存在，不回传原值）；
	//   2. 日志、错误信息、配置导出中一律脱敏；
	//   3. 与 Type=FieldPassword 的区别是：Secret 更强调「服务端不回传」。
	Secret bool `json:"secret,omitempty"`
}

// TypeOrDefault 返回字段类型，未指定时按文本处理。
func (f Field) TypeOrDefault() FieldType {
	if f.Type == "" {
		return FieldText
	}
	return f.Type
}

// Schema 一段配置的结构声明。
type Schema struct {
	// Title 表单标题。
	Title string `json:"title,omitempty"`
	// Description 表单顶部说明。
	Description string `json:"description,omitempty"`
	// Fields 字段列表，顺序即界面上的显示顺序。
	Fields []Field `json:"fields"`
}

// Field 按 Key 取字段声明。
func (s Schema) Field(key string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// Validate 对给定的配置值做基本校验：必填、类型、枚举。
//
// 只做「不用联网就能判断」的检查。需要真连一下服务商才知道对错的，
// 请由组件自己提供单独的「测试连接」接口。
func (s Schema) Validate(values map[string]any) error {
	for _, f := range s.Fields {
		v, ok := values[f.Key]
		if !ok || v == nil || v == "" {
			if f.Required {
				return fmt.Errorf("%s 不能为空", labelOf(f))
			}
			continue
		}

		switch f.TypeOrDefault() {
		case FieldBool:
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s 应为布尔值", labelOf(f))
			}
		case FieldNumber:
			switch v.(type) {
			case float64, float32, int, int64:
			default:
				return fmt.Errorf("%s 应为数字", labelOf(f))
			}
		case FieldSelect:
			want, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s 应为字符串", labelOf(f))
			}
			found := false
			for _, o := range f.Options {
				if o.Value == want {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s 的取值 %q 不在允许范围内", labelOf(f), want)
			}
		}
	}
	return nil
}

// Redact 生成脱敏后的配置副本，用于写日志、导出配置、回显给前端。
//
// 规则：所有 Secret 字段与非空的 Password 字段，值一律替换为 "***"；
// 值为空则保留空串，这样前端能区分「没配」和「配了」。
func (s Schema) Redact(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		out[k] = v
	}
	for _, f := range s.Fields {
		v, ok := out[f.Key]
		if !ok || v == nil || v == "" {
			continue
		}
		if f.Secret || f.TypeOrDefault() == FieldPassword {
			out[f.Key] = "***"
		}
	}
	return out
}

func labelOf(f Field) string {
	if f.Label != "" {
		return f.Label
	}
	return f.Key
}
