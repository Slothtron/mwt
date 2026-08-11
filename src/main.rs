use std::process::ExitCode;

fn main() -> ExitCode {
    // 占位:阶段 E 接 clap 派生的 Cli 后,改为:
    //   let cli = Cli::parse();
    //   match mwt::run(cli) { Ok(()) => ExitCode::SUCCESS, Err(e) => { eprintln!("error: {e:#}"); ExitCode::FAILURE } }
    eprintln!("mwt binary is not yet implemented (stage E pending)");
    ExitCode::FAILURE
}
