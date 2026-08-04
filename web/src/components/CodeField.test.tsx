import { beforeAll, describe, expect, it } from "vitest";
import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import CodeField, { formatCode } from "./CodeField";

// jsdom 缺 matchMedia，antd 组件需要。
beforeAll(() => {
  window.matchMedia =
    window.matchMedia ||
    ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }));
});

// 等一帧：编辑器经 requestAnimationFrame 挂载（展开/收起后 host 重建、由 rAF 重新挂载 CodeMirror）。
const nextFrame = () => new Promise((r) => setTimeout(r, 40));

describe("CodeField 格式化", () => {
  it("可以格式化 JavaScript", async () => {
    await expect(
      formatCode("const value={a:1,b:[2,3]}", "javascript"),
    ).resolves.toContain("const value = { a: 1, b: [2, 3] }");
  });

  it("可以格式化 JSON", async () => {
    await expect(
      formatCode('{"name":"baboflow","enabled":true}', "json"),
    ).resolves.toContain('"name": "baboflow"');
  });

  it("不格式化不支持的语言", async () => {
    await expect(formatCode("select * from user", "sql")).resolves.toBe(
      "select * from user",
    );
  });

  it("格式错误时返回拒绝结果", async () => {
    await expect(formatCode("const =", "javascript")).rejects.toBeTruthy();
  });
});

// 受控容器：模拟 NodeConfigPanel 的 Form，把 CodeField 的 onChange 灌回 value。
function Controlled({ spy }: { spy: string[] }) {
  const [v, setV] = useState("return msg.a + 1;");
  return (
    <CodeField
      value={v}
      language="javascript"
      onChange={(x) => {
        setV(x);
        spy.push(x);
      }}
    />
  );
}

describe("CodeField 展开/收起不丢内容", () => {
  it("初始挂载显示内容", async () => {
    render(<Controlled spy={[]} />);
    await nextFrame();
    expect(document.querySelector(".cm-content")?.textContent).toContain(
      "return msg.a + 1;",
    );
  });

  it("展开后编辑器仍在且内容保留（不空白）", async () => {
    render(<Controlled spy={[]} />);
    await nextFrame();
    fireEvent.click(screen.getByLabelText("展开编辑器"));
    await nextFrame();
    const cmInModal = document.querySelector(".ant-modal .cm-content");
    expect(cmInModal).not.toBeNull();
    expect(cmInModal?.textContent).toContain("return msg.a + 1;");
  });

  it("展开→收起后内容保留", async () => {
    render(<Controlled spy={[]} />);
    await nextFrame();
    fireEvent.click(screen.getByLabelText("展开编辑器"));
    await nextFrame();
    fireEvent.click(screen.getByLabelText("收起编辑器"));
    await nextFrame();
    expect(document.querySelector(".cm-content")?.textContent).toContain(
      "return msg.a + 1;",
    );
  });
});
