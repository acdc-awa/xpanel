# -*- coding: utf-8 -*-
"""Scan master/web/src for unused exports and unused components."""
import os, re, json, sys
from collections import defaultdict

SRC = r"E:\XrayProject\master\web\src"
EXCLUDE_DIRS = {"node_modules", "dist", ".git"}
EXCLUDE_FILES = {"auto-imports.d.ts", "components.d.ts"}

# collect files
files = []
for root, dirs, names in os.walk(SRC):
    dirs[:] = [d for d in dirs if d not in EXCLUDE_DIRS]
    for n in names:
        if n.endswith((".ts", ".vue", ".tsx")) and n not in EXCLUDE_FILES:
            files.append(os.path.join(root, n))

contents = {f: open(f, encoding="utf-8", errors="replace").read() for f in files}

def kebab(name):
    s = re.sub(r"(.)([A-Z][a-z]+)", r"\1-\2", name)
    return re.sub(r"([a-z0-9])([A-Z])", r"\1-\2", s).lower()

export_re = re.compile(
    r"^\s*export\s+(?:async\s+)?(?:const|let|var|function|class|type|interface|enum)\s+([A-Za-z_$][\w$]*)",
    re.M)
brace_re = re.compile(r"^\s*export\s*\{([^}]*)\}", re.M)

exports = defaultdict(list)  # name -> [file]
for f, text in contents.items():
    for m in export_re.finditer(text):
        exports[m.group(1)].append(f)
    for m in brace_re.finditer(text):
        for part in m.group(1).split(","):
            part = part.strip()
            if not part:
                continue
            name = part.split(" as ")[0].strip()
            if name:
                exports[name].append(f)

word_cache = None
def count_refs(name, defining_files):
    """count word-boundary refs across all files, excluding pure definition matches"""
    hits = []
    pat = re.compile(r"(?<![\w$])" + re.escape(name) + r"(?![\w$])")
    for f, text in contents.items():
        for m in pat.finditer(text):
            line_no = text.count("\n", 0, m.start())
            line = text.splitlines()[line_no].strip()
            # skip the defining export statement itself
            if f in defining_files and re.match(r"^export\s+(async\s+)?(const|let|var|function|class|type|interface|enum)\s+" + re.escape(name) + r"\b", line):
                continue
            if f in defining_files and re.match(r"^export\s*\{", line):
                continue
            hits.append((f, line_no + 1, line[:120]))
    return hits

dead, internal_only = [], []
for name, defs in sorted(exports.items()):
    refs = [h for h in count_refs(name, set(defs)) if h[0] not in defs]
    internal = [h for h in count_refs(name, set(defs)) if h[0] in defs]
    if not refs and not internal:
        dead.append((name, defs, []))
    elif not refs:
        internal_only.append((name, defs, internal[:2]))

print("== A. 零引用导出（完全死代码）==")
for name, defs, _ in dead:
    print(f"  {name}  <- {', '.join(os.path.relpath(d, SRC) for d in defs)}")

print("\n== B. 仅本文件内部使用（过度导出，非死代码）==")
for name, defs, sample in internal_only:
    print(f"  {name}  <- {', '.join(os.path.relpath(d, SRC) for d in defs)}")

# vue components usage check
print("\n== C. components/ 下的 .vue 使用情况 ==")
vue_files = [f for f in files if f.endswith(".vue")]
comp_dir = os.path.join(SRC, "components")
for vf in vue_files:
    if not vf.startswith(comp_dir):
        continue
    base = os.path.splitext(os.path.basename(vf))[0]
    k = kebab(base)
    pat = re.compile(r"(?<![\w$-])(" + re.escape(base) + r"|" + re.escape(k) + r")(?![\w-])")
    used = False
    for f, text in contents.items():
        if f == vf:
            continue
        if pat.search(text):
            used = True
            break
    if not used:
        print(f"  未被引用: {os.path.relpath(vf, SRC)}")

# views referenced by router?
print("\n== D. views/ 引用情况（router 或手动 import）==")
router_text = ""
for f in files:
    if "router" in os.path.basename(f).lower() or os.sep + "router" + os.sep in f:
        router_text += contents[f]
for vf in vue_files:
    if os.sep + "views" + os.sep not in vf:
        continue
    base = os.path.basename(vf)
    referenced = base in router_text or os.path.splitext(base)[0] in router_text
    # also check dynamic import anywhere
    if not referenced:
        for f, text in contents.items():
            if base in text and f != vf:
                referenced = True
                break
    if not referenced:
        print(f"  疑似未挂路由: {os.path.relpath(vf, SRC)}")
