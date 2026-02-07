use reqwest::header::HeaderMap;

pub(crate) fn next_page_from_link_header(headers: &HeaderMap) -> Option<u32> {
    let link = headers.get("link")?.to_str().ok()?;
    for part in link.split(',') {
        let part = part.trim();
        if !part.contains("rel=\"next\"") {
            continue;
        }
        let start = part.find('<')? + 1;
        let end = part.find('>')?;
        let url = &part[start..end];
        for pair in url.split('?').nth(1).unwrap_or("").split('&') {
            let mut iter = pair.splitn(2, '=');
            let key = iter.next().unwrap_or("");
            let value = iter.next().unwrap_or("");
            if key == "page"
                && let Ok(page) = value.parse::<u32>()
            {
                return Some(page);
            }
        }
    }
    None
}

pub(crate) fn next_page_from_header(headers: &HeaderMap, name: &str) -> Option<u32> {
    headers
        .get(name)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.parse::<u32>().ok())
}

pub(crate) fn string_header(headers: &HeaderMap, name: &str) -> Option<String> {
    headers
        .get(name)
        .and_then(|value| value.to_str().ok())
        .map(ToString::to_string)
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;

    #[test]
    fn next_page_from_link_header_parses_page() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "link",
            HeaderValue::from_static(
                "<https://api.github.com/orgs/test/repos?per_page=100&page=4>; rel=\"next\"",
            ),
        );
        assert_eq!(next_page_from_link_header(&headers), Some(4));
    }

    #[test]
    fn next_page_from_header_parses_u32() {
        let mut headers = HeaderMap::new();
        headers.insert("x-next-page", HeaderValue::from_static("3"));
        assert_eq!(next_page_from_header(&headers, "x-next-page"), Some(3));
    }
}
