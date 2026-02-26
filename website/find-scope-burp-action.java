// 1. Get the requested hostname from Burp's requestResponse object
String requestHost = requestResponse.httpService().host();

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
    String targetsString = programMatcher.group(2); // Looks like: "https://target1.com","https://target2.com"

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
                // Root Domain / Subdomain check:
                if (requestHost.equalsIgnoreCase(targetHost) || requestHost.endsWith("." + targetHost)) {
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