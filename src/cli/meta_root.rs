//! `mwt meta-root` — hidden helper that prints the meta-root resolved by
//! walking up for `.mwt.yaml`. Mirrors Go's `newRootInfoCmd`.

use std::io::Write;

use crate::cli::deps::Deps;

pub fn run(deps: &Deps, out: &mut dyn Write) -> Result<(), String> {
    let cfg = (deps.load_config)().map_err(|e| e.to_string())?;
    writeln!(out, "{}", cfg.meta_root.display()).map_err(|e| e.to_string())
}
