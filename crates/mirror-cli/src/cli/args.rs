use super::*;
#[derive(Parser)]
#[command(author, version, about)]
pub(super) struct Cli {
    #[arg(
        long,
        global = true,
        help = "Check for updates before running the command"
    )]
    pub(super) check_updates: bool,
    #[command(subcommand)]
    pub(super) command: Commands,
}

#[derive(clap::Subcommand)]
pub(super) enum Commands {
    #[command(about = "Manage config")]
    Config(ConfigArgs),
    #[command(about = "Manage targets")]
    Target(TargetArgs),
    #[command(about = "Manage auth tokens")]
    Token(TokenArgs),
    #[command(about = "Sync repos")]
    Sync(SyncArgs),
    #[command(about = "Run the daemon loop")]
    Daemon(DaemonArgs),
    #[command(about = "Install or uninstall background service helpers (placeholder)")]
    Service(ServiceArgs),
    #[command(about = "Validate provider auth and scope")]
    Health(HealthArgs),
    #[command(about = "Manage webhooks")]
    Webhook(WebhookArgs),
    #[command(about = "Manage cache")]
    Cache(CacheArgs),
    #[command(about = "Launch terminal UI")]
    Tui(TuiArgs),
    #[command(about = "Install daemon and optionally register PATH")]
    Install(InstallArgs),
    #[command(about = "Manage scheduled task (Windows only)")]
    Task(TaskArgs),
    #[command(about = "Check for updates and install latest release")]
    Update(UpdateArgs),
}

#[derive(Parser)]
pub(super) struct ConfigArgs {
    #[command(subcommand)]
    pub(super) command: ConfigCommands,
}

#[derive(clap::Subcommand)]
pub(super) enum ConfigCommands {
    #[command(about = "Initialize config with a mirror root path")]
    Init(InitArgs),
}

#[derive(Parser)]
pub(super) struct InitArgs {
    #[arg(long)]
    pub(super) root: PathBuf,
}

#[derive(Parser)]
pub(super) struct TargetArgs {
    #[command(subcommand)]
    pub(super) command: TargetCommands,
}

#[derive(clap::Subcommand)]
pub(super) enum TargetCommands {
    #[command(about = "Add a provider target to the config")]
    Add(AddTargetArgs),
    #[command(about = "List configured targets")]
    List,
    #[command(about = "Remove a target by id")]
    Remove(RemoveTargetArgs),
}

#[derive(Parser)]
pub(super) struct AddTargetArgs {
    #[arg(long, value_enum)]
    pub(super) provider: ProviderKindValue,
    #[arg(long, required = true)]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) host: Option<String>,
    #[arg(long, value_delimiter = ',')]
    pub(super) label: Vec<String>,
}

#[derive(Parser)]
pub(super) struct RemoveTargetArgs {
    #[arg(long)]
    pub(super) id: String,
}

#[derive(Parser)]
pub(super) struct TokenArgs {
    #[command(subcommand)]
    pub(super) command: TokenCommands,
}

#[derive(clap::Subcommand)]
pub(super) enum TokenCommands {
    #[command(about = "Store an auth token for a provider target")]
    Set(SetTokenArgs),
    #[command(about = "Show PAT guidance for a provider")]
    Guide(GuideTokenArgs),
    #[command(about = "Validate token scopes when supported")]
    Validate(ValidateTokenArgs),
    #[command(about = "Diagnose keyring/session issues for token storage")]
    Doctor(DoctorTokenArgs),
}

#[derive(Parser)]
pub(super) struct SetTokenArgs {
    #[arg(long, value_enum)]
    pub(super) provider: ProviderKindValue,
    #[arg(long, required = true)]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) host: Option<String>,
    #[arg(long)]
    pub(super) token: String,
}

#[derive(Parser)]
pub(super) struct GuideTokenArgs {
    #[arg(long, value_enum)]
    pub(super) provider: ProviderKindValue,
    #[arg(long, required = true)]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) host: Option<String>,
}

#[derive(Parser)]
pub(super) struct ValidateTokenArgs {
    #[arg(long, value_enum)]
    pub(super) provider: ProviderKindValue,
    #[arg(long, required = true)]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) host: Option<String>,
}

#[derive(Parser)]
pub(super) struct DoctorTokenArgs {
    #[arg(long, value_enum)]
    pub(super) provider: Option<ProviderKindValue>,
    #[arg(long)]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) host: Option<String>,
}

#[derive(Parser)]
pub(super) struct SyncArgs {
    #[arg(
        long,
        help = "Target id selector (takes precedence over --provider/--scope)"
    )]
    pub(super) target_id: Option<String>,
    #[arg(
        long,
        value_enum,
        help = "Provider selector (used when --target-id is not set)"
    )]
    pub(super) provider: Option<ProviderKindValue>,
    #[arg(
        long,
        help = "Provider scope selector segments; requires --provider unless --target-id is set"
    )]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) repo: Option<String>,
    #[arg(long)]
    pub(super) refresh: bool,
    #[arg(
        long,
        help = "Force a full refresh across all configured targets/repos; ignores --target-id/--provider/--scope/--repo"
    )]
    pub(super) force_refresh_all: bool,
    #[arg(long)]
    pub(super) include_archived: bool,
    #[arg(long)]
    pub(super) verify: bool,
    #[arg(long)]
    pub(super) status: bool,
    #[arg(long)]
    pub(super) status_only: bool,
    #[arg(long)]
    pub(super) audit_repo: bool,
    #[arg(long, default_value = "1")]
    pub(super) jobs: usize,
    #[arg(long)]
    pub(super) non_interactive: bool,
    #[arg(long, value_enum, default_value = "prompt")]
    pub(super) missing_remote: MissingRemotePolicyValue,
    #[arg(long)]
    pub(super) config: Option<PathBuf>,
    #[arg(long)]
    pub(super) cache: Option<PathBuf>,
}

#[derive(Parser)]
pub(super) struct DaemonArgs {
    #[arg(long)]
    pub(super) lock: Option<PathBuf>,
    #[arg(long, default_value = "3600")]
    pub(super) interval_seconds: u64,
    #[arg(long)]
    pub(super) run_once: bool,
    #[arg(long)]
    pub(super) audit_repo: bool,
    #[arg(long, default_value = "1")]
    pub(super) jobs: usize,
    #[arg(long, value_enum, default_value = "skip")]
    pub(super) missing_remote: MissingRemotePolicyValue,
    #[arg(long)]
    pub(super) config: Option<PathBuf>,
    #[arg(long)]
    pub(super) cache: Option<PathBuf>,
}

#[derive(Parser)]
pub(super) struct ServiceArgs {
    #[arg(value_enum)]
    pub(super) action: ServiceAction,
}

#[derive(Parser)]
pub(super) struct WebhookArgs {
    #[command(subcommand)]
    pub(super) command: WebhookCommands,
}

#[derive(clap::Subcommand)]
pub(super) enum WebhookCommands {
    #[command(about = "Register a webhook for a provider target")]
    Register(WebhookRegisterArgs),
}

#[derive(Parser)]
pub(super) struct WebhookRegisterArgs {
    #[arg(long, value_enum)]
    pub(super) provider: ProviderKindValue,
    #[arg(long, required = true)]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) host: Option<String>,
    #[arg(long)]
    pub(super) url: String,
    #[arg(long)]
    pub(super) secret: Option<String>,
}

#[derive(Parser)]
pub(super) struct CacheArgs {
    #[command(subcommand)]
    pub(super) command: CacheCommands,
}

#[derive(clap::Subcommand)]
pub(super) enum CacheCommands {
    #[command(about = "Prune cache entries for missing targets")]
    Prune(CachePruneArgs),
    #[command(about = "Show cached repo overview tree")]
    Overview(CacheOverviewArgs),
}

#[derive(Parser)]
pub(super) struct CachePruneArgs {
    #[arg(long)]
    pub(super) config: Option<PathBuf>,
    #[arg(long)]
    pub(super) cache: Option<PathBuf>,
}

#[derive(Parser)]
pub(super) struct CacheOverviewArgs {
    #[arg(long)]
    pub(super) config: Option<PathBuf>,
    #[arg(long)]
    pub(super) cache: Option<PathBuf>,
    #[arg(long)]
    pub(super) refresh: bool,
}

#[derive(Parser)]
pub(super) struct HealthArgs {
    #[arg(
        long,
        help = "Target id selector (takes precedence over --provider/--scope)"
    )]
    pub(super) target_id: Option<String>,
    #[arg(
        long,
        value_enum,
        help = "Provider selector (used when --target-id is not set)"
    )]
    pub(super) provider: Option<ProviderKindValue>,
    #[arg(
        long,
        help = "Provider scope selector segments; requires --provider unless --target-id is set"
    )]
    pub(super) scope: Vec<String>,
    #[arg(long)]
    pub(super) config: Option<PathBuf>,
}

#[derive(Parser)]
pub(super) struct TuiArgs {
    #[arg(long)]
    pub(super) dashboard: bool,
    #[arg(long)]
    pub(super) install: bool,
}

#[derive(Parser)]
pub(super) struct InstallArgs {
    #[arg(long)]
    pub(super) tui: bool,
    #[arg(long)]
    pub(super) status: bool,
    #[arg(long)]
    pub(super) start: bool,
    #[arg(long)]
    pub(super) update: bool,
    #[arg(long)]
    pub(super) delayed_start: Option<u64>,
    #[arg(long, value_enum)]
    pub(super) path: Option<PathChoiceValue>,
    #[arg(long)]
    pub(super) non_interactive: bool,
}

#[derive(Parser)]
pub(super) struct UpdateArgs {
    #[arg(
        long = "check-only",
        alias = "check",
        help = "Check for updates without installing"
    )]
    pub(super) check_only: bool,
    #[arg(long)]
    pub(super) apply: bool,
    #[arg(long)]
    pub(super) yes: bool,
    #[arg(long)]
    pub(super) repo: Option<String>,
}

#[derive(Clone, Copy, ValueEnum)]
pub(super) enum PathChoiceValue {
    Add,
    Skip,
}

impl From<PathChoiceValue> for PathChoice {
    fn from(value: PathChoiceValue) -> Self {
        match value {
            PathChoiceValue::Add => PathChoice::Add,
            PathChoiceValue::Skip => PathChoice::Skip,
        }
    }
}

#[derive(Clone, Copy, ValueEnum)]
pub(super) enum ServiceAction {
    Install,
    Uninstall,
}

#[derive(Clone, Copy, ValueEnum)]
pub(super) enum ProviderKindValue {
    AzureDevOps,
    GitHub,
    GitLab,
}

impl From<ProviderKindValue> for ProviderKind {
    fn from(value: ProviderKindValue) -> Self {
        match value {
            ProviderKindValue::AzureDevOps => ProviderKind::AzureDevOps,
            ProviderKindValue::GitHub => ProviderKind::GitHub,
            ProviderKindValue::GitLab => ProviderKind::GitLab,
        }
    }
}

#[derive(Clone, Copy, ValueEnum, PartialEq, Eq)]
pub(super) enum MissingRemotePolicyValue {
    Prompt,
    Archive,
    Remove,
    Skip,
}

impl From<MissingRemotePolicyValue> for MissingRemotePolicy {
    fn from(value: MissingRemotePolicyValue) -> Self {
        match value {
            MissingRemotePolicyValue::Prompt => MissingRemotePolicy::Prompt,
            MissingRemotePolicyValue::Archive => MissingRemotePolicy::Archive,
            MissingRemotePolicyValue::Remove => MissingRemotePolicy::Remove,
            MissingRemotePolicyValue::Skip => MissingRemotePolicy::Skip,
        }
    }
}

#[derive(Parser)]
pub(super) struct TaskArgs {
    #[command(subcommand)]
    pub(super) command: TaskCommands,
}

#[derive(clap::Subcommand)]
pub(super) enum TaskCommands {
    #[command(about = "Show scheduled task status")]
    Status,
    #[command(about = "Run scheduled task now")]
    Run,
    #[command(about = "Remove scheduled task registration")]
    Remove,
}
