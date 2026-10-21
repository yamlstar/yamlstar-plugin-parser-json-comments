(ns yamlstar-plugin.parser.json-comments-test
  (:require [clojure.test :refer [deftest is testing run-tests]]
            [clojure.edn :as edn]
            [clojure.string :as str]
            [yaml-parser.core :as reference]
            [yamlstar-plugin.parser.json-comments :as parser]))

(deftest fixtures
  (doseq [{:keys [name input want error]}
          (edn/read-string (slurp "testdata/cases.edn"))]
    (testing name
      (if error
        (try (parser/parse input) (is false "expected parse error")
             (catch Exception e (is (str/includes? (ex-message e) error))))
        (is (= (reference/parse want) (parser/parse input)))))))

(deftest parser-isolation
  (is (= "true// text" (:value (nth (reference/parse "true// text") 2))))
  (is (= "true" (:value (nth (parser/parse "true// text") 2))))
  (is (= "true// text" (:value (nth (reference/parse "true// text") 2)))))

(defn -main [& _]
  (let [{:keys [fail error]} (run-tests 'yamlstar-plugin.parser.json-comments-test)]
    (System/exit (+ fail error))))
