#!/usr/bin/env python3

import json
import os
import sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
os.chdir(ROOT)


def load_json(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


required_files = [
    "docs/INTENT_REQUIREMENTS_BRAIN.json",
    "docs/SEMANTIC_KNOWLEDGE.json",
    "docs/SCHEMA_INDEX.json",
    "docs/TEST_MAP.json",
    "docs/RUNTIME_WORKER_BRAIN.json",
]

missing = [p for p in required_files if not os.path.exists(p)]

if missing:
    print("FAIL: missing required knowledge files:")
    for p in missing:
        print(" -", p)
    sys.exit(1)


requirements = load_json(
    "docs/INTENT_REQUIREMENTS_BRAIN.json"
)["requirements"]

semantic = load_json(
    "docs/SEMANTIC_KNOWLEDGE.json"
)

schema = load_json(
    "docs/SCHEMA_INDEX.json"
)

tests = load_json(
    "docs/TEST_MAP.json"
)

runtime = load_json(
    "docs/RUNTIME_WORKER_BRAIN.json"
)


rows = []


for req in requirements:

    req_id = req["id"]
    area = req["area"]
    code_paths = req.get("code", [])

    # ---------------------------------------------------------
    # CODE FILES
    # ---------------------------------------------------------

    files = []

    for f in semantic.get("files", []):

        file_path = f["file"]

        for base in code_paths:

            base_clean = base.rstrip("/")

            if (
                file_path == base_clean
                or file_path.startswith(base_clean + "/")
            ):
                files.append(file_path)
                break

    files = sorted(set(files))

    # ---------------------------------------------------------
    # SYMBOLS
    # ---------------------------------------------------------

    symbols = []

    for s in semantic.get("symbols", []):

        if s.get("file") in files:

            symbols.append(
                {
                    "name": s.get("symbol"),
                    "kind": s.get("kind"),
                    "file": s.get("file"),
                    "line": s.get("start_line"),
                }
            )

    # ---------------------------------------------------------
    # DATABASE TABLES
    # ---------------------------------------------------------

    tables = set()

    for s in semantic.get("symbols", []):

        if s.get("file") not in files:
            continue

        for table in s.get("sql_tables", []):
            tables.add(table)

    tables = sorted(tables)

    # ---------------------------------------------------------
    # MIGRATIONS
    # ---------------------------------------------------------

    migrations = set(req.get("migrations", []))

    for table_name in tables:

        for table in schema.get("tables", []):

            if table.get("table") != table_name:
                continue

            created = table.get("created_by")

            if created:
                migrations.add(created)

            for change in table.get("altered_by", []):
                migration = change.get("migration")

                if migration:
                    migrations.add(migration)

    migrations = sorted(migrations)

    # ---------------------------------------------------------
    # TEST FILES
    # ---------------------------------------------------------

    test_files = set()

    for test in tests.get("test_files", []):

        test_path = test.get("file", "")

        for base in code_paths:

            base_clean = base.rstrip("/")

            if (
                test_path == base_clean
                or test_path.startswith(base_clean + "/")
            ):
                test_files.add(test_path)
                break

        # match foo.go -> foo_test.go
        for source_file in files:

            if not source_file.endswith(".go"):
                continue

            expected = source_file[:-3] + "_test.go"

            if test_path == expected:
                test_files.add(test_path)

    test_files = sorted(test_files)

    # ---------------------------------------------------------
    # RUNTIME EVIDENCE
    # ---------------------------------------------------------

    runtime_evidence = []

    for event in runtime.get("runtime_events", []):

        if event.get("file") in files:
            runtime_evidence.append(event)

    # ---------------------------------------------------------
    # COVERAGE
    # ---------------------------------------------------------

    coverage = {
        "code": bool(files),
        "symbols": bool(symbols),
        "database": bool(tables),
        "migrations": bool(migrations),
        "tests": bool(test_files),
        "runtime": bool(runtime_evidence),
    }

    rows.append(
        {
            "requirement_id": req_id,
            "area": area,
            "requirement": req["requirement"],
            "rationale": req.get("rationale", ""),
            "code_paths": code_paths,
            "files": files,
            "symbols": symbols,
            "tables": tables,
            "migrations": migrations,
            "tests": test_files,
            "runtime_evidence": runtime_evidence,
            "coverage": coverage,
        }
    )


# -------------------------------------------------------------
# SUMMARY
# -------------------------------------------------------------

summary = {
    "requirements": len(rows),
    "with_code": sum(bool(x["files"]) for x in rows),
    "with_symbols": sum(bool(x["symbols"]) for x in rows),
    "with_database": sum(bool(x["tables"]) for x in rows),
    "with_migrations": sum(bool(x["migrations"]) for x in rows),
    "with_tests": sum(bool(x["tests"]) for x in rows),
    "with_runtime": sum(bool(x["runtime_evidence"]) for x in rows),
}


output = {
    "schema_version": 1,
    "summary": summary,
    "rows": rows,
}


with open(
    "docs/TRACEABILITY_MATRIX.json",
    "w",
    encoding="utf-8",
) as f:

    json.dump(
        output,
        f,
        ensure_ascii=False,
        indent=2,
    )

    f.write("\n")


# -------------------------------------------------------------
# HUMAN READABLE VERSION
# -------------------------------------------------------------

md = []

md.append("# Requirement Traceability Matrix")
md.append("")

md.append(
    "Requirement → Code → Symbol → DB → Migration → Test → Runtime"
)

md.append("")

md.append(
    "| ID | Area | Files | Symbols | DB | Migrations | Tests | Runtime |"
)

md.append(
    "|---|---|---:|---:|---:|---:|---:|---:|"
)


for row in rows:

    md.append(
        "| `{}` | {} | {} | {} | {} | {} | {} | {} |".format(
            row["requirement_id"],
            row["area"],
            len(row["files"]),
            len(row["symbols"]),
            len(row["tables"]),
            len(row["migrations"]),
            len(row["tests"]),
            len(row["runtime_evidence"]),
        )
    )


md.append("")
md.append("## Coverage summary")
md.append("")

for key, value in summary.items():
    md.append(f"- **{key}**: {value}")


md.append("")
md.append("## Requirement details")
md.append("")


for row in rows:

    md.append(
        f"### {row['requirement_id']} — {row['area']}"
    )

    md.append("")
    md.append(row["requirement"])
    md.append("")

    if row["rationale"]:
        md.append(
            "**Why:** " + row["rationale"]
        )
        md.append("")

    if row["files"]:
        md.append("**Code:**")
        for item in row["files"]:
            md.append(f"- `{item}`")
        md.append("")

    if row["tables"]:
        md.append("**Database:**")
        for item in row["tables"]:
            md.append(f"- `{item}`")
        md.append("")

    if row["migrations"]:
        md.append("**Migrations:**")
        for item in row["migrations"]:
            md.append(f"- `{item}`")
        md.append("")

    if row["tests"]:
        md.append("**Tests:**")
        for item in row["tests"]:
            md.append(f"- `{item}`")
        md.append("")


with open(
    "docs/TRACEABILITY_MATRIX.md",
    "w",
    encoding="utf-8",
) as f:

    f.write("\n".join(md))
    f.write("\n")


print()
print("======================================")
print(" TRACEABILITY MATRIX GENERATED")
print("======================================")

for key, value in summary.items():
    print(f"{key}: {value}")

print()

