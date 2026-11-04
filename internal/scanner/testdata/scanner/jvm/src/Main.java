class Main {
    // System.getenv("FAKE_IN_COMMENT")
    static String url = System.getenv("REAL_JDBC_URL");
    static String text = """
        System.getenv("FAKE_IN_TEXT_BLOCK")
        """;
    static String dyn(String k) { return System.getenv(k); }
}
