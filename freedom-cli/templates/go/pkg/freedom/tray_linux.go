//go:build linux

package freedom

/*
#cgo pkg-config: gtk+-3.0
#cgo CFLAGS: -Wno-deprecated-declarations
#include <gtk/gtk.h>

extern void goTrayClick(void);
extern void goTrayMenuItemActivate(int idx);

// GtkStatusIcon 自 GTK3.14 起标记 deprecated，但 legacy tray 协议在 KDE/XFCE/
// WSLg 仍广泛可用；AppIndicator(GDBus) 适配在未来自成一档。
static int ensureGtk(void) { return gtk_init_check(NULL, NULL) ? 1 : 0; }

static void onTrayActivate(GtkStatusIcon *icon, gpointer data) {
	(void)icon; (void)data;
	goTrayClick();
}

// 右键：弹出前端注册的菜单（popup-menu 信号），返回 TRUE 抑制默认行为。
static gboolean onTrayPopupMenu(GtkStatusIcon *icon, guint button, guint activate_time, gpointer data) {
	(void)button; (void)activate_time; (void)data;
	GtkWidget *menu = (GtkWidget*) g_object_get_data(G_OBJECT(icon), "freedom-menu");
	if (menu != NULL) gtk_menu_popup_at_pointer(GTK_MENU(menu), NULL);
	return TRUE;
}

static void onMenuItemClick(GtkMenuItem *item, gpointer data) {
	(void)item;
	goTrayMenuItemActivate(GPOINTER_TO_INT(data));
}

static GtkStatusIcon* trayNewFromPng(const guint8 *png, gsize len) {
	GdkPixbufLoader *loader = gdk_pixbuf_loader_new();
	gboolean ok = gdk_pixbuf_loader_write(loader, png, len, NULL);
	ok = ok && gdk_pixbuf_loader_close(loader, NULL);
	GdkPixbuf *pb = ok ? gdk_pixbuf_loader_get_pixbuf(loader) : NULL;
	GtkStatusIcon *icon = pb ? gtk_status_icon_new_from_pixbuf(pb) : NULL;
	g_object_unref(loader);
	if (icon == NULL) return NULL;
	g_signal_connect(icon, "activate", G_CALLBACK(onTrayActivate), NULL);
	g_signal_connect(icon, "popup-menu", G_CALLBACK(onTrayPopupMenu), NULL);
	gtk_status_icon_set_visible(icon, TRUE);
	return icon;
}

static GtkStatusIcon* trayNewNamed(const char *name) {
	GtkStatusIcon *icon = gtk_status_icon_new_from_icon_name(name);
	g_signal_connect(icon, "activate", G_CALLBACK(onTrayActivate), NULL);
	g_signal_connect(icon, "popup-menu", G_CALLBACK(onTrayPopupMenu), NULL);
	gtk_status_icon_set_visible(icon, TRUE);
	return icon;
}

static void traySetTooltip(GtkStatusIcon *icon, const char *tip) {
	gtk_status_icon_set_tooltip_text(icon, tip);
}

static void trayDestroy(GtkStatusIcon *icon) {
	gtk_status_icon_set_visible(icon, FALSE);
	g_object_unref(icon);
}

// 菜单所有权：attach 时 ref_sink + set_data_full，重复 attach 自动释放旧菜单。
static void trayAttachMenu(GtkStatusIcon *icon, GtkWidget *menu) {
	g_object_ref_sink(menu);
	g_object_set_data_full(G_OBJECT(icon), "freedom-menu", menu, (GDestroyNotify) g_object_unref);
	gtk_widget_show_all(menu);
}

static GtkWidget* menuNew(void) { return gtk_menu_new(); }

static GtkWidget* menuSubmenu(GtkWidget *parent, const char *label) {
	GtkWidget *item = gtk_menu_item_new_with_label(label);
	GtkWidget *sub = gtk_menu_new();
	gtk_menu_item_set_submenu(GTK_MENU_ITEM(item), sub);
	gtk_menu_shell_append(GTK_MENU_SHELL(parent), item);
	return sub;
}

static void menuAppendItem(GtkWidget *menu, const char *label, int idx, gboolean sensitive) {
	GtkWidget *item = gtk_menu_item_new_with_label(label);
	gtk_widget_set_sensitive(item, sensitive);
	g_signal_connect(item, "activate", G_CALLBACK(onMenuItemClick), GINT_TO_POINTER(idx));
	gtk_menu_shell_append(GTK_MENU_SHELL(menu), item);
}

static void menuAppendCheck(GtkWidget *menu, const char *label, int idx, gboolean checked, gboolean sensitive) {
	GtkWidget *item = gtk_check_menu_item_new_with_label(label);
	gtk_check_menu_item_set_active(GTK_CHECK_MENU_ITEM(item), checked);
	gtk_widget_set_sensitive(item, sensitive);
	g_signal_connect(item, "activate", G_CALLBACK(onMenuItemClick), GINT_TO_POINTER(idx));
	gtk_menu_shell_append(GTK_MENU_SHELL(menu), item);
}

static void menuAppendSeparator(GtkWidget *menu) {
	gtk_menu_shell_append(GTK_MENU_SHELL(menu), gtk_separator_menu_item_new());
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

// M4 Linux 托盘（GtkStatusIcon，cgo）：与 Windows 侧 trayCall 同一前端契约
// （tray.create/destroy/tooltip/menu；事件 tray:click / tray:menu({id})）。
// 限制：GtkStatusIcon 无双击信号（tray:double-click 不触发）；menu.set（主窗口
// 原生菜单栏）与 GDBus StatusNotifierItem 适配未实装，托盘图标走 legacy
// system tray 协议（KDE/xfce/wmctrl 系支持；GNOME 需 AppIndicator 扩展）。
// GTK 调用须发生在 main线程：__freedom_tray 桥回调经 WebKit 消息处理器在
// GTK main loop 线程执行，天然满足；单测须先 gtk_init_check 且有 DISPLAY。

type linuxTrayState struct {
	mu    sync.Mutex
	icon  *C.GtkStatusIcon
	items map[int32]string // 菜单条目序号 → 前端 id
	emit  func(event string, data interface{})
}

var linuxTray = &linuxTrayState{items: map[int32]string{}}

//export goTrayClick
func goTrayClick() {
	linuxTray.mu.Lock()
	emit := linuxTray.emit
	linuxTray.mu.Unlock()
	if emit != nil {
		emit("tray:click", map[string]interface{}{"button": "left"})
	}
}

//export goTrayMenuItemActivate
func goTrayMenuItemActivate(idx C.int) {
	linuxTray.mu.Lock()
	id := linuxTray.items[int32(idx)]
	emit := linuxTray.emit
	linuxTray.mu.Unlock()
	if emit != nil {
		emit("tray:menu", map[string]interface{}{"id": id})
	}
}

// trayCall 处理前端 __freedom_tray 请求（Linux 实现，方法面与 Windows 对齐）。
func (a *App) trayCall(method string, paramsJSON string) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("freedom: tray method %q panicked: %v", method, r)
		}
	}()

	var args map[string]json.RawMessage
	if len(paramsJSON) > 0 && paramsJSON != "null" {
		if err := json.Unmarshal([]byte(paramsJSON), &args); err != nil {
			return nil, fmt.Errorf("freedom: tray method %q: invalid args: %w", method, err)
		}
	}
	argStr := func(k string) string {
		if v, ok := args[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
		return ""
	}

	switch method {
	case "tray.create":
		return nil, linuxTrayCreate(a, argStr("icon"), argStr("tooltip"))
	case "tray.destroy":
		linuxTrayDestroy()
		return nil, nil
	case "tray.tooltip":
		linuxTray.mu.Lock()
		icon := linuxTray.icon
		linuxTray.mu.Unlock()
		if icon == nil {
			return nil, fmt.Errorf("tray: 托盘未创建")
		}
		tip := C.CString(argStr("tooltip"))
		C.traySetTooltip(icon, tip)
		C.free(unsafe.Pointer(tip))
		return nil, nil
	case "tray.menu":
		linuxTray.mu.Lock()
		icon := linuxTray.icon
		linuxTray.mu.Unlock()
		if icon == nil {
			return nil, fmt.Errorf("tray: 托盘未创建")
		}
		var items []menuItem
		if v, ok := args["items"]; ok {
			if err := json.Unmarshal(v, &items); err != nil {
				return nil, fmt.Errorf("tray: 菜单格式错误: %w", err)
			}
		}
		linuxTraySetMenu(icon, items)
		return nil, nil
	case "menu.set":
		return nil, fmt.Errorf("freedom: menu.set 在 linux 暂不支持（GTK 菜单栏待补）")
	default:
		return nil, fmt.Errorf("freedom: unknown tray method %q", method)
	}
}

func linuxTrayCreate(a *App, iconDataURL, tooltip string) error {
	if C.ensureGtk() == 0 {
		return fmt.Errorf("freedom: 无可用显示服务（DISPLAY/Wayland），无法创建托盘")
	}
	png := dataURLToBytes(iconDataURL)
	var icon *C.GtkStatusIcon
	if len(png) > 0 {
		icon = C.trayNewFromPng((*C.guint8)(unsafe.Pointer(&png[0])), C.gsize(len(png)))
	} else {
		name := C.CString("application-x-executable")
		icon = C.trayNewNamed(name)
		C.free(unsafe.Pointer(name))
	}
	if icon == nil {
		return fmt.Errorf("freedom: 托盘图标创建失败（PNG 无法解析或无托盘协议支持）")
	}
	if tooltip != "" {
		tip := C.CString(tooltip)
		C.traySetTooltip(icon, tip)
		C.free(unsafe.Pointer(tip))
	}
	linuxTray.mu.Lock()
	old := linuxTray.icon
	linuxTray.icon = icon
	linuxTray.emit = a.Emit
	linuxTray.mu.Unlock()
	if old != nil {
		C.trayDestroy(old)
	}
	return nil
}

func linuxTrayDestroy() {
	linuxTray.mu.Lock()
	icon := linuxTray.icon
	linuxTray.icon = nil
	linuxTray.items = map[int32]string{}
	linuxTray.mu.Unlock()
	if icon != nil {
		C.trayDestroy(icon)
	}
}

// linuxTraySetMenu 重建右键菜单并接管 icon 上的旧菜单释放（set_data_full）。
func linuxTraySetMenu(icon *C.GtkStatusIcon, items []menuItem) {
	linuxTray.mu.Lock()
	linuxTray.items = map[int32]string{}
	var next int32
	assign := func(id string) int32 {
		h := next
		next++
		linuxTray.items[h] = id
		return h
	}
	var build func(menu *C.GtkWidget, list []menuItem)
	build = func(menu *C.GtkWidget, list []menuItem) {
		for _, it := range list {
			label := C.CString(it.Label)
			switch {
			case it.Type == "separator":
				C.menuAppendSeparator(menu)
			case it.Type == "submenu" || len(it.Submenu) > 0:
				sub := C.menuSubmenu(menu, label)
				// 父条目无动作，仅展开；子条目递归编号。
				assign(it.ID)
				build(sub, it.Submenu)
			default:
				idx := assign(it.ID)
				sensitive := C.gboolean(1)
				if it.Enabled != nil && !*it.Enabled {
					sensitive = 0
				}
				if it.Checked != nil {
					checked := C.gboolean(0)
					if *it.Checked {
						checked = 1
					}
					C.menuAppendCheck(menu, label, C.int(idx), checked, sensitive)
				} else {
					C.menuAppendItem(menu, label, C.int(idx), sensitive)
				}
			}
			C.free(unsafe.Pointer(label))
		}
	}
	menu := C.menuNew()
	build(menu, items)
	C.trayAttachMenu(icon, menu)
	linuxTray.mu.Unlock()
}
