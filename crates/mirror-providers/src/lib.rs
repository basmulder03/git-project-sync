pub mod auth;
pub mod azure_devops;
pub mod github;
pub mod gitlab;
mod http;
pub mod registry;
pub mod spec;
pub use mirror_core::provider::RepoProvider;
pub use registry::ProviderRegistry;
