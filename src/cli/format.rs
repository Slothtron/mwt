//! `--format <table|json>` renderer for the list / doctor / path subcommands.
//!
//! 与 Go 版 inline 的 `formatList` / `formatListJSON` 对应;`json` 包统一
//! 用 `serde_json` 输出结构化数据(plan v2)。

use std::io::{self, Write};

use serde::Serialize;

/// Closed enum for `--format`. Rejects any other value at parse time.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, clap::ValueEnum)]
pub enum OutputFormat {
    #[default]
    Table,
    Json,
}

impl std::str::FromStr for OutputFormat {
    type Err = String;
    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "table" => Ok(Self::Table),
            "json" => Ok(Self::Json),
            other => Err(format!(
                "invalid format {other:?}: expected one of \"table\", \"json\""
            )),
        }
    }
}

impl OutputFormat {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Table => "table",
            Self::Json => "json",
        }
    }
}

/// Single `mwt list` row.
#[derive(Debug, Clone, Serialize)]
pub struct ListRow {
    pub repo: String,
    pub branch: String,
    pub path: String,
}

/// Render list rows to `w` in the chosen format. Empty input is a no-op.
pub fn write_list(w: &mut dyn Write, fmt: OutputFormat, rows: &[ListRow]) -> io::Result<()> {
    match fmt {
        OutputFormat::Table => write_list_table(w, rows),
        OutputFormat::Json => write_list_json(w, rows),
    }
}

fn write_list_table(w: &mut dyn Write, rows: &[ListRow]) -> io::Result<()> {
    if rows.is_empty() {
        return Ok(());
    }
    let h_repo = "REPO";
    let h_branch = "BRANCH";
    let h_path = "PATH";
    let max_repo = rows
        .iter()
        .map(|r| r.repo.len())
        .max()
        .unwrap_or(0)
        .max(h_repo.len());
    let max_branch = rows
        .iter()
        .map(|r| r.branch.len())
        .max()
        .unwrap_or(0)
        .max(h_branch.len());
    writeln!(w, "{h_repo:<max_repo$}  {h_branch:<max_branch$}  {h_path}")?;
    for r in rows {
        writeln!(
            w,
            "{:<max_repo$}  {:<max_branch$}  {}",
            r.repo, r.branch, r.path
        )?;
    }
    Ok(())
}

fn write_list_json(w: &mut dyn Write, rows: &[ListRow]) -> io::Result<()> {
    let json = serde_json::to_string_pretty(rows).map_err(io::Error::other)?;
    writeln!(w, "{json}")
}

/// Render a single `mwt path` result. `path` is a string, not a structured
/// object — the JSON form is `{ "path": "..." }` for symmetry with list/doctor.
pub fn write_path(w: &mut dyn Write, fmt: OutputFormat, path: &str) -> io::Result<()> {
    match fmt {
        OutputFormat::Table => writeln!(w, "{path}"),
        OutputFormat::Json => {
            let v = serde_json::json!({ "path": path });
            let s = serde_json::to_string_pretty(&v).map_err(io::Error::other)?;
            writeln!(w, "{s}")
        }
    }
}

/// Doctor findings serialization. We re-use `crate::doctor::Finding`'s shape
/// and add a top-level array wrapper.
pub fn write_doctor(
    w: &mut dyn Write,
    fmt: OutputFormat,
    findings: &[crate::doctor::Finding],
) -> io::Result<()> {
    match fmt {
        OutputFormat::Table => crate::doctor::format_report(w, findings),
        OutputFormat::Json => {
            // Serialize the public Finding shape (kebab-case kind for parity
            // with the human table).
            let v: Vec<serde_json::Value> = findings
                .iter()
                .map(|f| {
                    serde_json::json!({
                        "kind": f.kind.as_str(),
                        "repo": f.repo,
                        "branch": f.branch,
                        "path": f.path,
                        "message": f.message,
                        "suggest": f.suggest,
                    })
                })
                .collect();
            let s = serde_json::to_string_pretty(&v).map_err(io::Error::other)?;
            writeln!(w, "{s}")
        }
    }
}
