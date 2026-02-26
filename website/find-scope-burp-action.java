// 1. Get the requested hostname from Burp's requestResponse object
String requestHost = requestResponse.httpService().host();

// --- ROOT DOMAIN EXTRACTOR ---
// This safely extracts the base domain (e.g., "codebig2.net" from "sub.codebig2.net")
// and accounts for common compound TLDs (like "example.co.uk").
java.util.function.Function<String, String> extractRoot = (host) -> {
    if (host == null) return "";
    String[] parts = host.split("\\.");
    if (parts.length <= 2) return host;
    
    String tld = parts[parts.length - 1];
    String sld = parts[parts.length - 2];
    
    // Check for common compound TLDs (e.g., .co.uk, .com.au)
    if ((tld.length() == 2 && (sld.equals("co") || sld.equals("com") || sld.equals("org") || sld.equals("net"))) || (sld.length() <= 3 && tld.length() == 2)) {
        if (parts.length >= 3) {
            return parts[parts.length - 3] + "." + sld + "." + tld;
        }
    }
    return sld + "." + tld;
};

String requestRoot = extractRoot.apply(requestHost);
// -----------------------------

// 2. Fetch the JSON from BBScope
StringBuilder jsonResponse = new StringBuilder();
try {
    java.net.URL url = new java.net.URL("https://bbscope.com/api/v1/programs");
    java.net.HttpURLConnection conn = (java.net.HttpURLConnection) url.openConnection();
    conn.setRequestMethod("GET");
    conn.setRequestProperty("User-Agent", "BurpSuite-BBScope-Action");
    
    java.io.BufferedReader in = new java.io.BufferedReader(new java.io.InputStreamReader(conn.getInputStream()));
    String inputLine;
    while ((inputLine = in.readLine()) != null) {
        jsonResponse.append(inputLine);
    }
    in.close();
} catch (Exception e) {
    logging().logToOutput("Error fetching JSON: " + e.getMessage());
}

String json = jsonResponse.toString();
String foundProgramUrl = null;

// 3. Parse the JSON using Regex
java.util.regex.Pattern programPattern = java.util.regex.Pattern.compile("\"url\":\"([^\"]+)\"[^}]+?\"targets\":\\[(.*?)\\]");
java.util.regex.Matcher programMatcher = programPattern.matcher(json);

while (programMatcher.find()) {
    String programUrl = programMatcher.group(1);
    String targetsString = programMatcher.group(2);

    // Extract individual target URLs from the array
    java.util.regex.Pattern targetUrlPattern = java.util.regex.Pattern.compile("\"([^\"]+)\"");
    java.util.regex.Matcher targetUrlMatcher = targetUrlPattern.matcher(targetsString);

    while (targetUrlMatcher.find()) {
        String rawTargetUrl = targetUrlMatcher.group(1);
        try {
            // Parse the host out of the target URL
            java.net.URI uri = new java.net.URI(rawTargetUrl);
            String targetHost = uri.getHost();
            
            if (targetHost != null) {
                String targetRoot = extractRoot.apply(targetHost);
                
                // Broad Check: Do they share the same root domain?
                // e.g., requestRoot (codebig2.net) == targetRoot (codebig2.net)
                if (requestRoot.equalsIgnoreCase(targetRoot)) {
                    foundProgramUrl = programUrl;
                    break;
                }
            }
        } catch (Exception e) {
            // Ignore malformed URIs inside the JSON target list
            continue; 
        }
    }
    
    // Stop searching if we already found a match
    if (foundProgramUrl != null) {
        break; 
    }
}

// 4. Output the results
if (foundProgramUrl != null) {
    logging().logToOutput("[+] BBP MATCH: " + requestHost + " belongs to -> " + foundProgramUrl);
}