// Freedom 进程后端示例：Node.js 实现（纯 Node 标准库，无需 npm 依赖）。
//
// 协议与 Go 后端完全一致：stdin 接收请求、stdout 返回响应/推送事件。
// 壳通过 `node backends/node_backend.mjs` 启动本进程。
import readline from "node:readline";

const rl = readline.createInterface({ input: process.stdin, terminal: false });

// stdin EOF（壳关闭 stdin）即优雅退出：setInterval 会保持事件循环存活，
// 不显式退出会让壳侧每次关闭都等满超时后被强杀。
rl.on("close", () => process.exit(0));

// 定时推送 tick 事件。
let count = 0;
setInterval(() => {
  count++;
  process.stdout.write(
    JSON.stringify({
      event: "tick",
      data: { time: new Date().toTimeString().slice(0, 8), count, lang: "Node" },
    }) + "\n"
  );
}, 2000);

rl.on("line", (line) => {
  let resp;
  try {
    const req = JSON.parse(line);
    let result = null;
    let error = "";
    switch (req.method) {
      case "Greet": {
        const [name] = req.params ?? [];
        if (!name) error = "名字不能为空";
        else result = `Hello, ${name}! (Node backend)`;
        break;
      }
      case "Add": {
        const [a, b] = req.params ?? [];
        result = a + b;
        break;
      }
      case "WhoAmI":
        result = "Node";
        break;
      default:
        error = "unknown method " + req.method;
    }
    resp = { id: req.id, result, error };
  } catch (e) {
    resp = { id: 0, error: String(e) };
  }
  process.stdout.write(JSON.stringify(resp) + "\n");
});
