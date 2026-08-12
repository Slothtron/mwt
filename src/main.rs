use std::process::ExitCode;

use clap::Parser;

fn main() -> ExitCode {
    let cli = mwt::cli::Cli::parse();
    mwt::run(cli)
}
