package freedom

// menuItem 描述一个托盘/菜单栏条目（JSON 透传）。M4 起为跨平台公共结构
// （Windows 托盘/菜单栏与 Linux GTK 托盘共用；原声明在 tray_windows.go）。
type menuItem struct {
	ID      string     `json:"id"`
	Label   string     `json:"label"`
	Type    string     `json:"type"`    // "item"（默认）/ "separator" / "submenu"
	Enabled *bool      `json:"enabled"` // nil=默认启用
	Checked *bool      `json:"checked"`
	Submenu []menuItem `json:"submenu"`
}
