#!/usr/bin/env python3

import json
import os
import re
import subprocess

ROOT = os.path.abspath(
    os.path.join(os.path.dirname(__file__), "..")
)
os.chdir(ROOT)


def load(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


HEAD = subprocess.check_output(
    ["git", "rev-parse", "HEAD"],
    text=True,
).strip()


required = [
    "docs/SCHEMA_INDEX.json",
    "docs/LIVE_DB_BRAIN.json",
    "docs/FUNCTION_BEHAVIOR_BRAIN.json",
    "docs/FULL_SOURCE_KNOWLEDGE.json",
]

missing = [
    path for path in required
    if not os.path.exists(path)
]

if missing:
    print("FAIL: missing required knowledge files")

    for path in missing:
        print(" -", path)

    raise SystemExit(1)


schema = load("docs/SCHEMA_INDEX.json")
live = load("docs/LIVE_DB_BRAIN.json")
functions = load("docs/FUNCTION_BEHAVIOR_BRAIN.json")
source = load("docs/FULL_SOURCE_KNOWLEDGE.json")


# ---------------------------------------------------------
# Build exact source map from commit-pinned snapshot
# ---------------------------------------------------------

source_lines = {}

for file_info in source.get("files", []):

    path = file_info.get("path", "")

    if not path.endswith(".go"):
        continue

    source_lines[path] = [
        item.get("text", "")
        for item in file_info.get("lines", [])
    ]


# ---------------------------------------------------------
# Source schema map
# ---------------------------------------------------------

source_tables = {
    table["table"]: table
    for table in schema.get("tables", [])
}


# ---------------------------------------------------------
# Helper
# ---------------------------------------------------------

def unique_refs(items):

    seen = set()
    result = []

    for item in items:

        key = (
            item.get("name"),
            item.get("file"),
            item.get("line"),
        )

        if key in seen:
            continue

        seen.add(key)
        result.append(item)

    return result


# ---------------------------------------------------------
# Build column knowledge
# ---------------------------------------------------------

columns = []


for live_table in live.get("tables", []):

    table_name = live_table["name"]

    source_table = source_tables.get(
        table_name,
        {},
    )

    source_columns = source_table.get(
        "columns",
        {},
    )

    for column in live_table.get("columns", []):

        column_name = column["name"]

        readers = []
        writers = []
        mentions = []

        column_pattern = re.compile(
            r"\b" + re.escape(column_name) + r"\b",
            re.IGNORECASE,
        )

        table_pattern = re.compile(
            r"\b" + re.escape(table_name) + r"\b",
            re.IGNORECASE,
        )

        for fn in functions.get("functions", []):

            file_path = fn.get("file")

            lines = source_lines.get(file_path)

            if not lines:
                continue

            start = fn.get("start_line", 1)
            end = fn.get("end_line", start)

            if start < 1:
                continue

            body = "\n".join(
                lines[start - 1:end]
            )

            if not column_pattern.search(body):
                continue

            ref = {
                "name": fn.get("name"),
                "file": file_path,
                "line": start,
            }

            mentions.append(ref)

            # ---------------------------------------------
            # Conservative SQL read detection
            # ---------------------------------------------

            has_table = bool(
                table_pattern.search(body)
            )

            if has_table:

                read_sql = re.search(
                    r"(?is)"
                    r"\b(?:SELECT|FROM|JOIN)\b"
                    r".*?"
                    + r"\b"
                    + re.escape(table_name)
                    + r"\b",
                    body,
                )

                if read_sql:
                    readers.append(ref)

                # -----------------------------------------
                # Conservative SQL write detection
                # -----------------------------------------

                write_sql = re.search(
                    r"(?is)"
                    r"\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\b"
                    r".*?"
                    + r"\b"
                    + re.escape(table_name)
                    + r"\b",
                    body,
                )

                if write_sql:
                    writers.append(ref)


        readers = unique_refs(readers)
        writers = unique_refs(writers)
        mentions = unique_refs(mentions)


        # -------------------------------------------------
        # Migration origin
        # -------------------------------------------------

        source_column = source_columns.get(
            column_name,
            {},
        )

        added_by = source_column.get(
            "added_by"
        )

        definition = source_column.get(
            "definition"
        )


        columns.append(
            {
                "table": table_name,
                "column": column_name,

                "data_type": column.get(
                    "type"
                ),

                "nullable": column.get(
                    "nullable"
                ),

                "default": column.get(
                    "default"
                ),

                "added_by": added_by,

                "definition": definition,

                "reader_functions": readers,

                "writer_functions": writers,

                "mention_functions": mentions,
            }
        )


# ---------------------------------------------------------
# Statistics
# ---------------------------------------------------------

with_readers = sum(
    bool(item["reader_functions"])
    for item in columns
)

with_writers = sum(
    bool(item["writer_functions"])
    for item in columns
)

with_mentions = sum(
    bool(item["mention_functions"])
    for item in columns
)

with_origin = sum(
    bool(item["added_by"])
    for item in columns
)


output = {
    "schema_version": 1,

    "source_commit": HEAD,

    "scope": (
        "live production public schema metadata enriched "
        "with commit-pinned static source/migration evidence; "
        "contains no application row data or secrets"
    ),

    "table_count": len(
        live.get("tables", [])
    ),

    "column_count": len(columns),

    "statistics": {
        "columns_with_reader_functions": with_readers,
        "columns_with_writer_functions": with_writers,
        "columns_with_function_mentions": with_mentions,
        "columns_with_migration_origin": with_origin,
    },

    "columns": columns,
}


with open(
    "docs/COLUMN_BEHAVIOR_BRAIN.json",
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


# ---------------------------------------------------------
# Human-readable summary
# ---------------------------------------------------------

md = []

md.append("# Database Column Behavior Brain")
md.append("")

md.append(
    f"Commit `{HEAD}`."
)

md.append("")

md.append(
    f"Live production columns indexed: **{len(columns)}**"
)

md.append(
    f"Live production tables: **{len(live.get('tables', []))}**"
)

md.append("")

md.append(
    f"- Columns with reader evidence: **{with_readers}**"
)

md.append(
    f"- Columns with writer evidence: **{with_writers}**"
)

md.append(
    f"- Columns with function mentions: **{with_mentions}**"
)

md.append(
    f"- Columns with migration-origin evidence: **{with_origin}**"
)

md.append("")

md.append(
    "Each entry combines live PostgreSQL schema metadata "
    "with commit-pinned migration/source evidence."
)

md.append("")

md.append(
    "Reader/writer classification is conservative static SQL evidence. "
    "Dynamic SQL, repository indirection, generated queries, or aliases "
    "may require exact source inspection."
)

md.append("")

md.append(
    "No application rows, DATABASE_URL, credentials, tokens, "
    "passwords, cookies, or secrets are stored."
)


with open(
    "docs/COLUMN_BEHAVIOR_BRAIN.md",
    "w",
    encoding="utf-8",
) as f:

    f.write("\n".join(md))
    f.write("\n")


print()
print("========================================")
print(" COLUMN BRAIN GENERATED")
print("========================================")

print("source_commit:", HEAD)
print("tables:", len(live.get("tables", [])))
print("columns:", len(columns))
print("reader evidence:", with_readers)
print("writer evidence:", with_writers)
print("function mentions:", with_mentions)
print("migration origins:", with_origin)

