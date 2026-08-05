#!/usr/bin/env python3

import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCE_DIRS = (ROOT / "internal/server", ROOT / "internal/service")
ROUTE_METHODS = {
    "GET",
    "POST",
    "PUT",
    "DELETE",
    "PATCH",
    "OPTIONS",
    "HEAD",
    "Any",
    "Handle",
    "Match",
    "Group",
    "Static",
    "StaticFile",
    "StaticFS",
}
ALLOWED_ROUTES = {
    ("internal/server/http.go", "GET", "/healthz"),
    ("internal/server/http.go", "GET", "/readyz"),
    ("internal/server/http.go", "GET", "/metrics"),
    ("internal/server/http.go", "GET", "/ws"),
    ("internal/server/http.go", "Any", "/mcp/sse"),
    ("internal/server/http.go", "Any", "/mcp/message"),
    ("internal/server/http.go", "Static", "/assets"),
    ("internal/server/http.go", "GET", "/api/v1/auth/feishu/login"),
    ("internal/server/http.go", "GET", "/api/v1/auth/feishu/callback"),
    ("internal/server/http.go", "POST", "/api/v1/agent-assets"),
    ("internal/server/http.go", "GET", "/api/v1/agent-assets/:assetId"),
    ("internal/server/http.go", "POST", "/api/v1/skills/package"),
    ("internal/server/http.go", "GET", "/api/v1/skills/:id/package"),
}
ALLOWED_GIN_CONTEXT_FUNCTIONS = {
    "internal/server/http.go": {"NewHTTPServer", "writeSidecarNotFound"},
    "internal/service/agent.go": {"agentErr", "UploadAsset", "GetAsset"},
    "internal/service/feishu.go": {"audit", "Login", "Callback", "redirectLoginErr"},
    "internal/service/mcp_auth.go": {"MCPAuthMiddleware"},
    "internal/service/middleware.go": {
        "sessionCookieContext",
        "setSessionCookie",
        "GinAuthMiddleware",
        "CurrentUserID",
        "ginError",
        "pathID",
    },
    "internal/service/skill.go": {"skillErr", "UploadPackage", "DownloadPackage"},
    "internal/service/ws.go": {"Handle"},
}
LEGACY_PATTERNS = (
    (re.compile(r"internal/server/httputil"), "legacy httputil import"),
    (re.compile(r"\bhttputil\.(?:OK|Fail|OKPage)\b"), "legacy httputil envelope"),
    (re.compile(r"\btype\s+(?:response|envelope)\s+struct\b"), "legacy envelope type"),
    (re.compile(r'gin\.H\s*\{\s*"code"\s*:'), "legacy Gin envelope"),
)
FUNC_PATTERN = re.compile(r"^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)")
CALL_PATTERN = re.compile(
    r"\.\s*(" + "|".join(sorted(ROUTE_METHODS, key=len, reverse=True)) + r")\s*\("
)


def production_go_files():
    for directory in SOURCE_DIRS:
        yield from sorted(
            path for path in directory.rglob("*.go") if not path.name.endswith("_test.go")
        )


def mask_comments(source):
    chars = list(source)
    i = 0
    state = "code"
    while i < len(chars):
        char = chars[i]
        next_char = chars[i + 1] if i + 1 < len(chars) else ""
        if state == "code":
            if char == "/" and next_char == "/":
                chars[i] = chars[i + 1] = " "
                state = "line_comment"
                i += 2
                continue
            if char == "/" and next_char == "*":
                chars[i] = chars[i + 1] = " "
                state = "block_comment"
                i += 2
                continue
            if char == '"':
                state = "string"
            elif char == "'":
                state = "rune"
            elif char == "`":
                state = "raw"
        elif state == "line_comment":
            if char == "\n":
                state = "code"
            else:
                chars[i] = " "
        elif state == "block_comment":
            if char == "*" and next_char == "/":
                chars[i] = chars[i + 1] = " "
                state = "code"
                i += 2
                continue
            if char != "\n":
                chars[i] = " "
        elif state in {"string", "rune"}:
            if char == "\\":
                i += 2
                continue
            if (state == "string" and char == '"') or (state == "rune" and char == "'"):
                state = "code"
        elif state == "raw" and char == "`":
            state = "code"
        i += 1
    return "".join(chars)


def split_arguments(source, open_paren):
    arguments = []
    start = open_paren + 1
    depth = 1
    state = "code"
    i = start
    while i < len(source):
        char = source[i]
        if state == "code":
            if char == '"':
                state = "string"
            elif char == "'":
                state = "rune"
            elif char == "`":
                state = "raw"
            elif char in "([{":
                depth += 1
            elif char in ")]}":
                depth -= 1
                if depth == 0:
                    arguments.append(source[start:i].strip())
                    return arguments
            elif char == "," and depth == 1:
                arguments.append(source[start:i].strip())
                start = i + 1
        elif state in {"string", "rune"}:
            if char == "\\":
                i += 2
                continue
            if (state == "string" and char == '"') or (state == "rune" and char == "'"):
                state = "code"
        elif state == "raw" and char == "`":
            state = "code"
        i += 1
    return None


def go_string(argument):
    argument = argument.strip()
    if len(argument) >= 2 and argument[0] == "`" and argument[-1] == "`":
        return argument[1:-1]
    if len(argument) >= 2 and argument[0] == '"' and argument[-1] == '"':
        try:
            return json.loads(argument)
        except json.JSONDecodeError:
            return None
    return None


def route_path(method, arguments):
    path_index = 1 if method in {"Handle", "Match"} else 0
    if len(arguments) <= path_index:
        return None
    return go_string(arguments[path_index])


def line_number(source, offset):
    return source.count("\n", 0, offset) + 1


def check_routes(relative, source, masked):
    errors = []
    for match in CALL_PATTERN.finditer(masked):
        method = match.group(1)
        arguments = split_arguments(masked, match.end() - 1)
        line = line_number(source, match.start())
        if arguments is None:
            errors.append(f"{relative}:{line}: unterminated .{method}( route call")
            continue
        path = route_path(method, arguments)
        if path is None:
            continue
        if (relative, method, path) not in ALLOWED_ROUTES:
            errors.append(
                f"{relative}:{line}: unapproved Gin route registration: {method} {path}"
            )
    return errors


def check_gin_context(relative, source, masked):
    errors = []
    current_function = None
    allowed = ALLOWED_GIN_CONTEXT_FUNCTIONS.get(relative, set())
    for number, (line, masked_line) in enumerate(
        zip(source.splitlines(), masked.splitlines()), start=1
    ):
        function_match = FUNC_PATTERN.match(masked_line)
        if function_match:
            current_function = function_match.group(1)
        if "gin.Context" in masked_line and current_function not in allowed:
            function_name = current_function or "<package scope>"
            errors.append(
                f"{relative}:{number}: unexpected gin.Context in {function_name}"
            )
    return errors


def main():
    errors = []
    for path in production_go_files():
        relative = path.relative_to(ROOT).as_posix()
        source = path.read_text(encoding="utf-8")
        masked = mask_comments(source)
        errors.extend(check_routes(relative, source, masked))
        errors.extend(check_gin_context(relative, source, masked))
        for pattern, description in LEGACY_PATTERNS:
            for match in pattern.finditer(masked):
                errors.append(
                    f"{relative}:{line_number(source, match.start())}: {description}"
                )
    if errors:
        for error in errors:
            print(f"guard: {error}", file=sys.stderr)
        return 1
    print("guard: repository boundaries verified")
    return 0


if __name__ == "__main__":
    sys.exit(main())
