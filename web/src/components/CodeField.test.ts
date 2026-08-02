import { describe, expect, it } from "vitest";

import { formatCode } from "./CodeField";

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
