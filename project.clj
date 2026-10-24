(defproject org.yamlstar/yamlstar-plugin-parser-json-comments "0.1.0"
  :description "JSON-style comments as YAML parser productions"
  :url "https://github.com/yamlstar/yamlstar-plugin-parser-json-comments"
  :license {:name "MIT"}
  :dependencies [[org.clojure/clojure "1.12.0"]
                 [org.yamlstar/yaml-parser "0.2.6"]]
  :source-paths ["src"]
  :deploy-repositories
  [["releases" {:url "https://repo.clojars.org"
                :username :env/clojars_username
                :password :env/clojars_password
                :sign-releases false}]])
