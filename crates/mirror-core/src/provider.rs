use crate::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo};
use anyhow::bail;
use std::cell::RefCell;
use std::future::Future;
use std::pin::Pin;
use tokio::runtime::{Builder, Runtime};

pub type ProviderFuture<'a, T> = Pin<Box<dyn Future<Output = anyhow::Result<T>> + 'a>>;

pub fn block_on<F: Future>(future: F) -> F::Output {
    thread_local! {
        static RUNTIME: RefCell<Option<Runtime>> = const { RefCell::new(None) };
    }

    RUNTIME.with(|cell| {
        let mut runtime = cell.borrow_mut();
        if runtime.is_none() {
            let created = Builder::new_current_thread()
                .enable_time()
                .build()
                .expect("create provider runtime");
            *runtime = Some(created);
        }
        runtime
            .as_mut()
            .expect("provider runtime initialized")
            .block_on(future)
    })
}

pub trait RepoProvider {
    fn kind(&self) -> ProviderKind;
    fn list_repos<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, Vec<RemoteRepo>>;
    fn validate_auth<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, ()>;
    fn health_check<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, ()> {
        Box::pin(async move { self.validate_auth(target).await })
    }
    fn auth_for_target<'a>(
        &'a self,
        _target: &'a ProviderTarget,
    ) -> ProviderFuture<'a, Option<crate::model::RepoAuth>> {
        Box::pin(async { Ok(None) })
    }
    fn token_scopes<'a>(
        &'a self,
        _target: &'a ProviderTarget,
    ) -> ProviderFuture<'a, Option<Vec<String>>> {
        Box::pin(async { Ok(None) })
    }
    fn register_webhook<'a>(
        &'a self,
        _target: &'a ProviderTarget,
        _url: &str,
        _secret: Option<&str>,
    ) -> ProviderFuture<'a, ()> {
        Box::pin(async { bail!("webhook registration not supported for this provider") })
    }

    fn get_repo<'a>(
        &'a self,
        _target: &'a ProviderTarget,
        _repo_id: &str,
    ) -> ProviderFuture<'a, Option<RemoteRepo>> {
        Box::pin(async { Ok(None) })
    }
}

pub trait ProviderSpec {
    fn kind(&self) -> ProviderKind;
    fn default_host(&self) -> &'static str;
    fn parse_scope(&self, segments: Vec<String>) -> anyhow::Result<ProviderScope>;
    fn validate_scope(&self, scope: &ProviderScope) -> anyhow::Result<()>;
    fn account_key(&self, host: &str, scope: &ProviderScope) -> anyhow::Result<String>;
}
