//! Format a doctor report for human consumption.

use std::fmt::Write as _;
use std::io::{self, Write};

use crate::doctor::{Finding, Kind};

/// Write a human-readable doctor report to `w`.
///
/// - Empty findings → single `ok: no issues found` line.
/// - Setup-missing present → one-shot `mwt doctor --fix` footer at the end.
pub fn format_report(w: &mut dyn Write, findings: &[Finding]) -> io::Result<()> {
    if findings.is_empty() {
        writeln!(w, "ok: no issues found")?;
        return Ok(());
    }

    let mut has_setup_missing = false;
    for (i, f) in findings.iter().enumerate() {
        if i > 0 {
            writeln!(w)?;
        }
        if f.kind == Kind::SetupMissing {
            has_setup_missing = true;
        }
        let mut header = format!("[{}]", f.kind.as_str());
        if !f.repo.is_empty() {
            let _ = write!(header, " {}", f.repo);
        }
        writeln!(w, "{header}: {}", f.message)?;
        if !f.branch.is_empty() {
            writeln!(w, "  branch: {}", f.branch)?;
        }
        if !f.suggest.is_empty() {
            writeln!(w, "  suggest:")?;
            for line in &f.suggest {
                writeln!(w, "    {line}")?;
            }
        }
    }
    if has_setup_missing {
        writeln!(w)?;
        writeln!(w, "to fix all setup_missing:")?;
        writeln!(w, "  mwt doctor --fix")?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn f(kind: Kind, repo: &str, branch: &str, path: &str, msg: &str) -> Finding {
        Finding::new(kind, repo, branch, path, msg)
    }

    #[test]
    fn empty_yields_ok() {
        let mut buf = Vec::new();
        format_report(&mut buf, &[]).unwrap();
        assert_eq!(String::from_utf8(buf).unwrap(), "ok: no issues found\n");
    }

    #[test]
    fn single_finding_without_suggest() {
        let mut buf = Vec::new();
        let fs = [f(
            Kind::MainMissing,
            "api",
            "",
            "/meta/api",
            "main checkout missing: /meta/api",
        )];
        format_report(&mut buf, &fs).unwrap();
        let out = String::from_utf8(buf).unwrap();
        assert!(out.contains("[main_missing] api"));
        assert!(out.contains("main checkout missing"));
    }

    #[test]
    fn setup_missing_triggers_fix_footer() {
        let mut buf = Vec::new();
        let fs = [f(Kind::SetupMissing, "api", "feat-x", "/wt", "missing")
            .with_suggest(["mwt setup feat-x --repos api"])];
        format_report(&mut buf, &fs).unwrap();
        let out = String::from_utf8(buf).unwrap();
        assert!(out.contains("to fix all setup_missing:"));
        assert!(out.contains("mwt doctor --fix"));
    }
}
