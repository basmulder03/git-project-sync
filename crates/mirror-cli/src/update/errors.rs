pub fn is_network_error(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        if let Some(reqwest_err) = cause.downcast_ref::<reqwest::Error>() {
            return reqwest_err.is_timeout()
                || reqwest_err.is_connect()
                || reqwest_err.is_request();
        }
        false
    })
}

pub fn is_permission_error(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        if let Some(io_err) = cause.downcast_ref::<std::io::Error>() {
            return io_err.kind() == std::io::ErrorKind::PermissionDenied;
        }
        false
    })
}
