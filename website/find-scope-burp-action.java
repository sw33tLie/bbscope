// 1. Get the requested hostname from Burp's requestResponse object
String requestHost = requestResponse.httpService().host();

// 2. Query the BBScope Find API
StringBuilder jsonResponse = new StringBuilder();
try {
    java.net.URL url = new java.net.URL("https://bbscope.com/api/v1/find?q=" + java.net.URLEncoder.encode(requestHost, "UTF-8"));
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
    logging().logToOutput("Error fetching from BBScope API: " + e.getMessage());
}

// 3. Extract program URLs from response
String json = jsonResponse.toString();
java.util.regex.Pattern urlPattern = java.util.regex.Pattern.compile("\"url\":\"([^\"]+)\"");
java.util.regex.Matcher urlMatcher = urlPattern.matcher(json);

java.util.List<String> programUrls = new java.util.ArrayList<>();
while (urlMatcher.find()) {
    programUrls.add(urlMatcher.group(1));
}

// 4. Output the results
if (!programUrls.isEmpty()) {
    for (String programUrl : programUrls) {
        logging().logToOutput("[+] BBP MATCH: " + requestHost + " belongs to -> " + programUrl);
    }
} else {
    logging().logToOutput("[-] No BBP match found for: " + requestHost);
}
