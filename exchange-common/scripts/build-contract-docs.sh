#!/bin/bash
# Contract docs build script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$COMMON_DIR")"
CONTRACTS_DIR="$REPO_ROOT/contracts"
VERSIONS_FILE="$CONTRACTS_DIR/versions.json"
DOCS_SOURCE_DIR="$REPO_ROOT/docs/contracts"

usage() {
    cat <<USAGE
Usage: $0 [--output DIR] [--dry-run]

Options:
  --output DIR  Output directory for generated docs (default: docs/contracts)
  --dry-run     Validate inputs without writing files
  -h, --help    Show this help message
USAGE
}

check_deps() {
    if ! command -v python3 &> /dev/null; then
        echo "Error: python3 not found." >&2
        exit 1
    fi
}

render_index() {
    local output_file="$1"

    python3 - "$VERSIONS_FILE" "$output_file" <<'PY'
import html
import json
import sys

versions_path = sys.argv[1]
out_path = sys.argv[2]

with open(versions_path, 'r', encoding='utf-8') as f:
    data = json.load(f)

current = data.get('current', '')
history = data.get('history', [])

current_entry = next((item for item in history if item.get('version') == current), {})

version_rows = []
for item in history:
    version = html.escape(item.get('version', ''))
    released_at = html.escape(item.get('released_at', ''))
    status = html.escape(item.get('status', ''))
    notes = html.escape(item.get('notes', ''))
    version_rows.append(
        """            <tr>
              <td><a href=\"/contracts/versions/{version}/\">{version}</a></td>
              <td>{released_at}</td>
              <td>{status}</td>
              <td>{notes}</td>
            </tr>""".format(
            version=version,
            released_at=released_at,
            status=status,
            notes=notes,
        )
    )

current_version = html.escape(current_entry.get('version', current))
current_status = html.escape(current_entry.get('status', ''))
current_released = html.escape(current_entry.get('released_at', ''))

html_content = """<!doctype html>
<html lang=\"en\">
  <head>
    <meta charset=\"utf-8\" />
    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />
    <title>Contract Docs</title>
    <link rel=\"stylesheet\" href=\"styles.css\" />
  </head>
  <body>
    <header>
      <div class=\"hero\">
        <span class=\"badge\">Versioned Contracts</span>
        <h1>Contract Documentation</h1>
        <p>
          Browse the current and historical REST/WS contracts with packaged OpenAPI and
          Proto sources. Each version keeps its own documentation, changelog, and error
          code reference.
        </p>
      </div>
    </header>
    <main>
      <section>
        <h2 class=\"section-title\">Current Release</h2>
        <div class=\"card\">
          <h3>{current_version}</h3>
          <p>Status: {current_status} · Released: {current_released}</p>
          <div class=\"links\">
            <a class=\"link-pill\" href=\"/contracts/versions/{current_version}/openapi/\">OpenAPI bundle</a>
            <a class=\"link-pill\" href=\"/contracts/versions/{current_version}/proto/\">Proto bundle</a>
            <a class=\"link-pill\" href=\"/contracts/versions/{current_version}/errors.md\">Error codes</a>
          </div>
        </div>
      </section>
      <section>
        <h2 class=\"section-title\">Quick Links</h2>
        <div class=\"card-grid\">
          <div class=\"card\">
            <h3>Release Notes</h3>
            <p>Track release narratives and compatibility guidance for every version.</p>
          </div>
          <div class=\"card\">
            <h3>Compatibility</h3>
            <p>Breaking-change checks run in CI before a version is published.</p>
          </div>
          <div class=\"card\">
            <h3>SDKs & Samples</h3>
            <p>SDK generation stays aligned with the published contract packages.</p>
          </div>
        </div>
      </section>
      <section>
        <h2 class=\"section-title\">Version History</h2>
        <table class=\"version-table\">
          <thead>
            <tr>
              <th>Version</th>
              <th>Released</th>
              <th>Status</th>
              <th>Notes</th>
            </tr>
          </thead>
          <tbody>
{rows}
          </tbody>
        </table>
      </section>
    </main>
    <footer>
      Generated from the contract version manifest. Documentation links resolve to the
      published contract bundles under /contracts/versions/.
    </footer>
  </body>
</html>
""".format(
    current_version=current_version,
    current_status=current_status,
    current_released=current_released,
    rows="\n".join(version_rows) if version_rows else "            <tr><td colspan=\"4\">No history available.</td></tr>",
)

with open(out_path, 'w', encoding='utf-8') as f:
    f.write(html_content)
PY
}

main() {
    local output_dir="$DOCS_SOURCE_DIR"
    local dry_run=false

    while [ "$#" -gt 0 ]; do
        case "$1" in
            --output)
                output_dir="$2"
                shift 2
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                echo "Unknown option: $1" >&2
                usage
                exit 1
                ;;
        esac
    done

    check_deps

    if [ ! -f "$VERSIONS_FILE" ]; then
        echo "Error: versions.json not found at $VERSIONS_FILE" >&2
        exit 1
    fi

    if [ ! -f "$DOCS_SOURCE_DIR/styles.css" ]; then
        echo "Error: docs styles.css not found at $DOCS_SOURCE_DIR/styles.css" >&2
        exit 1
    fi

    if [ "$dry_run" = true ]; then
        echo "OK"
        exit 0
    fi

    mkdir -p "$output_dir"
    render_index "$output_dir/index.html"

    if [ "$output_dir" != "$DOCS_SOURCE_DIR" ]; then
        cp "$DOCS_SOURCE_DIR/styles.css" "$output_dir/styles.css"
    fi

    echo "Done! Generated docs in $output_dir"
}

main "$@"
