(ns yamlstar-plugin.parser.json-comments
  "JSON-style comments implemented as YAML production replacements."
  (:require [yaml-parser.core :as core]
            [yaml-parser.canonical :as g]
            [yaml-parser.parser :as p]
            [yaml-parser.prelude :refer [p-get p-set!]]
            [yaml-parser.receiver :as receiver]))

(def version "0.1.0")

(defn- source [parser]
  #?(:glj (p-get parser :chars) :default (p-get parser :input)))

(defn- char-at [parser i]
  (when (< i (p-get parser :end)) (nth (source parser) i)))

(defn- starts? [parser i a b]
  (and (= a (char-at parser i)) (= b (char-at parser (inc i)))))

(defn- white? [ch] (or (= ch \space) (= ch \tab)))
(defn- break? [ch] (or (= ch \newline) (= ch \return)))

(defn- block-end [parser start]
  (loop [i (+ start 2)]
    (cond
      (>= i (p-get parser :end))
      (throw (ex-info "Unterminated block comment" {:position start}))
      (starts? parser i \* \/) (+ i 2)
      :else (recur (inc i)))))

(defn- line-end [parser start]
  (loop [i start]
    (if (or (>= i (p-get parser :end)) (break? (char-at parser i)))
      i (recur (inc i)))))

(defn- text [parser start end]
  #?(:glj (apply str (subvec (p-get parser :chars) start end))
     :default (subs (p-get parser :input) start end)))

(defn- json-literal? [value]
  (boolean (re-matches
             #"(?:null|true|false|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)"
             value)))

(defn- elide! [parser start end]
  (swap! (:comment-elisions (p-get parser :receiver)) assoc start end)
  end)

(defn- inline-separation [parser]
  (let [start (p-get parser :pos)]
    (loop [i start]
      (cond
        (white? (char-at parser i)) (recur (inc i))
        (starts? parser i \/ \*)
        (recur (elide! parser i (block-end parser i)))
        (starts? parser i \/ \/)
        (do (p-set! parser :pos (elide! parser i (line-end parser i))) true)
        :else
        (do (p-set! parser :pos i)
            (or (> i start) (p/start-of-line* parser)
                (and (= (char-at parser i) \#) (pos? i)
                     (contains? #{\" \' \, \] \}}
                                (char-at parser (dec i))))))))))

(defn- comment-text [parser]
  (let [i (p-get parser :pos)]
    (cond
      (or (= (char-at parser i) \#) (starts? parser i \/ \/))
      (do (p-set! parser :pos (line-end parser i)) true)
      (starts? parser i \/ \*)
      (do (p-set! parser :pos (block-end parser i)) true)
      :else false)))

(defn- marker? [parser]
  (let [i (p-get parser :pos)
        ch (char-at parser i)]
    (and (p/start-of-line* parser)
         (contains? #{\- \.} ch)
         (= ch (char-at parser (inc i)) (char-at parser (+ i 2)))
         (let [after (+ i 3)]
           (or (white? (char-at parser after))
               (break? (char-at parser after))
               (nil? (char-at parser after))
               (starts? parser after \/ \/)
               (starts? parser after \/ \*))))))

(defn- plain-char [parser c]
  (let [i (p-get parser :pos)]
    (when-not (or (marker? parser)
                   (and (or (zero? i)
                            (white? (char-at parser (dec i)))
                            (break? (char-at parser (dec i))))
                        (or (starts? parser i \/ \/)
                            (starts? parser i \/ \*))))
      (g/ns_plain_char parser c))))

(defn- block-indent [parser n]
  ;; A physical newline inside a header comment is not the header's end.
  (let [start (p-get parser :pos)
        newline (loop [i start]
                  (cond
                    (>= i (p-get parser :end)) i
                    (or (= (char-at parser i) \#)
                        (starts? parser i \/ \/)) (line-end parser i)
                    (starts? parser i \/ \*)
                    (recur (block-end parser i))
                    (break? (char-at parser i)) i
                    :else (recur (inc i))))]
    (try
      (p-set! parser :pos newline)
      (p/auto-detect parser n)
      (finally (p-set! parser :pos start)))))

(defn- indentation-indicator [parser n]
  (p/any parser
    (p/if* parser (p/rng parser "1" "9")
      (p/set* parser "m" (p/ord parser (p/match parser))))
    (p/if* parser (g/empty parser)
      (p/set* parser "m" [block-indent n]))))

(defn- plain-tail [parser context start elisions]
  (loop [end (p-get parser :pos)]
    (let [j (loop [i end]
              (if (white? (char-at parser i)) (recur (inc i)) i))
          comment? (or (starts? parser j \/ \*)
                       (starts? parser j \/ \/))
          adjacent-json (and comment? start (= j end)
                             (json-literal? (text parser start end)))]
      (cond
        (and comment? adjacent-json)
        (do (p-set! parser :pos end) true)

        (and comment? (> j end))
        (if (starts? parser j \/ \/)
          (do (p-set! parser :pos end) true)
          (let [after (block-end parser j)]
            (swap! elisions assoc j after)
            (p-set! parser :pos after)
            (let [tail (loop [i after]
                         (cond
                           (white? (char-at parser i)) (recur (inc i))
                           (starts? parser i \/ \*)
                           (let [q (block-end parser i)]
                             (swap! elisions assoc i q) (recur q))
                           :else i))]
              (p-set! parser :pos tail)
              (if (and (not (starts? parser tail \/ \/))
                       (not (= (char-at parser tail) \#))
                       (p/call parser [g/ns_plain_char context]))
                (recur (p-get parser :pos))
                (do (p-set! parser :pos end) true)))))

        :else
        (do (p-set! parser :pos j)
            (if (p/call parser [g/ns_plain_char context])
              (recur (p-get parser :pos))
              (do (p-set! parser :pos end) true)))))))

(defn- plain-line [elisions parser context]
  (let [start (p-get parser :pos)]
    (when (and (not (starts? parser start \/ \/))
               (not (starts? parser start \/ \*))
               (p/call parser [g/ns_plain_first context]))
      (plain-tail parser context start elisions))))

(defn- without-comments [raw start elisions]
  (if (empty? elisions)
    raw
    ;; Glojure strings are UTF-8; index a vector once rather than rescanning.
    (let [chars (vec raw)
          end (+ start (count chars))]
      (loop [i start parts []]
        (if (>= i end)
          (apply str parts)
          (if-let [after (get elisions i)]
            (recur (min after end) parts)
            (recur (inc i) (conj parts (nth chars (- i start))))))))))

(defn- receive-plain [r o]
  ((get receiver/receiver-callbacks "got__ns_plain") r
   (update o :text without-comments (:start o) @(:comment-elisions r))))

(defn parse
  "Parse original YAML/JSONC input and return YAML event maps."
  [input]
  (let [elisions (atom {})
        recv (-> (receiver/make-receiver-with-callbacks)
                 (assoc :comment-elisions elisions)
                 (assoc-in [:callbacks "got__ns_plain"] receive-plain))
        parser (core/make-parser recv
                 {:productions
                  {:s-separate-in-line inline-separation
                   :c-nb-comment-text comment-text
                   :c-indentation-indicator indentation-indicator
                   :c-forbidden marker?
                   :c-directives-end
                   (fn [parser]
                     (when (and (marker? parser)
                                (= \- (char-at parser (p-get parser :pos))))
                       (p-set! parser :pos (+ 3 (p-get parser :pos))) true))
                   :ns-plain-char plain-char
                   :ns-plain
                   (fn [parser n c]
                     (let [start (p-get parser :pos)
                           literal (loop [i start]
                                     (cond
                                       (or (starts? parser i \/ \/)
                                           (starts? parser i \/ \*))
                                       (json-literal? (text parser start i))
                                       (or (nil? (char-at parser i))
                                           (white? (char-at parser i))
                                           (break? (char-at parser i))) false
                                       :else (recur (inc i))))]
                       (if literal
                         (plain-line elisions parser c)
                         (g/ns_plain parser n c))))
                   :ns-plain-one-line (partial plain-line elisions)
                   :nb-ns-plain-in-line
                   (fn [parser c] (plain-tail parser c nil elisions))}})]
    (p/parse parser (or input ""))
    @(:events recv)))
