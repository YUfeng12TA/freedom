// Freedom 进程后端示例：Rust 实现（零第三方依赖，直接 rustc 编译）。
//
// 协议与 Go/Node/Python 后端完全一致：stdin 接收请求、stdout 返回响应/推送事件。
//
// 构建：rustc -O -o rust_backend.exe rust_backend.rs
//
// 本文件刻意不依赖 serde，用最小 JSON 子集解析器处理本框架的固定协议，
// 展示"任意语言后端"对语言/生态/依赖的零要求。
use std::io::{self, BufRead, Write};
use std::thread;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

fn main() {
    // 后台线程：每 2 秒向壳推送 tick 事件。
    thread::spawn(|| {
        let mut count: u64 = 0;
        loop {
            count += 1;
            let secs = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_secs();
            let line = format!(
                "{{\"event\":\"tick\",\"data\":{{\"time\":{},\"count\":{},\"lang\":\"Rust\"}}}}",
                secs, count
            );
            let mut so = io::stdout();
            let _ = writeln!(so, "{}", line);
            let _ = so.flush();
            thread::sleep(Duration::from_secs(2));
        }
    });

    let stdin = io::stdin();
    for line in stdin.lock().lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => break,
        };
        let line = line.trim().to_string();
        if line.is_empty() {
            continue;
        }
        let resp = handle(&line);
        let mut so = io::stdout();
        let _ = writeln!(so, "{}", resp);
        let _ = so.flush();
    }
}

fn handle(s: &str) -> String {
    let id = get_id(s).unwrap_or(0);
    let method = get_str_field(s, "method").unwrap_or("").to_string();
    let (p1, p2) = get_params(s);
    match method.as_str() {
        "Greet" => {
            if p1.is_empty() {
                format!("{{\"id\":{},\"error\":\"{}\"}}", id, "名字不能为空")
            } else {
                format!(
                    "{{\"id\":{},\"result\":\"Hello, {}! (Rust backend)\"}}",
                    id,
                    escape(&p1)
                )
            }
        }
        "Add" => {
            let a: f64 = p1.parse().unwrap_or(0.0);
            let b: f64 = p2.parse().unwrap_or(0.0);
            let sum = a + b;
            let num = if sum.fract() == 0.0 {
                format!("{}", sum as i64)
            } else {
                format!("{}", sum)
            };
            format!("{{\"id\":{},\"result\":{}}}", id, num)
        }
        "WhoAmI" => format!("{{\"id\":{},\"result\":\"Rust\"}}", id),
        _ => format!(
            "{{\"id\":{},\"error\":\"{}\"}}",
            id,
            escape(&format!("unknown method {}", method))
        ),
    }
}

/// 提取字符串字段 `"key":"value"` 的值。
fn get_str_field<'a>(s: &'a str, key: &str) -> Option<&'a str> {
    let pat = format!("\"{}\"", key);
    let pos = s.find(&pat)? + pat.len();
    let rest = s[pos..].trim_start_matches(|c: char| c == ':' || c.is_whitespace());
    let rest = rest.strip_prefix('"')?;
    let end = rest.find('"')?;
    Some(&rest[..end])
}

/// 提取数字字段 `"id":N` 的值。
fn get_id(s: &str) -> Option<i64> {
    let pos = s.find("\"id\"")? + 4;
    let rest = s[pos..].trim_start_matches(|c: char| c == ':' || c.is_whitespace());
    let end = rest.find(|c: char| c == ',' || c == '}')?;
    rest[..end].trim().parse().ok()
}

/// 解析 `"params":[...]` 数组的前两个元素（字符串或数字字面量）。
fn get_params(s: &str) -> (String, String) {
    let mut a = String::new();
    let mut b = String::new();
    let Some(pos) = s.find("\"params\"") else {
        return (a, b);
    };
    let mut rest = s[pos + 8..].trim_start_matches(|c: char| c == ':' || c.is_whitespace());
    if !rest.starts_with('[') {
        return (a, b);
    }
    rest = &rest[1..];
    let (v, r) = next_elem(rest);
    a = v;
    let (v2, _) = next_elem(r);
    b = v2;
    (a, b)
}

/// 解析数组中的下一个元素（字符串或数字），返回 (值, 剩余文本)。
fn next_elem(s: &str) -> (String, &str) {
    let s = s.trim_start().trim_start_matches(',');
    if let Some(rest) = s.strip_prefix('"') {
        if let Some(end) = rest.find('"') {
            return (rest[..end].to_string(), &rest[end + 1..]);
        }
        return (String::new(), "");
    }
    let end = s.find(|c: char| c == ',' || c == ']').unwrap_or(s.len());
    (s[..end].trim().to_string(), &s[end..])
}

/// 对 JSON 字符串值做转义（反斜杠、双引号、换行）。
fn escape(s: &str) -> String {
    s.replace('\\', "\\\\")
        .replace('"', "\\\"")
        .replace('\n', "\\n")
}
