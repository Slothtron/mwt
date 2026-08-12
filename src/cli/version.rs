//! `mwt version` — print build metadata.

use std::io::Write;

pub fn run(out: &mut dyn Write) -> Result<(), String> {
    writeln!(out, "{}", crate::version::string()).map_err(|e| e.to_string())
}
