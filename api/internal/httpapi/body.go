package httpapi

// Lecture des corps JSON et contrôle des noms, communs à la validation d'une
// ligne (étape 7) et à la création d'un profil (étape 7 bis).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxBodyBytes limite la taille d'un corps de requête. {"picker": "..."} ou
// {"name": "..."} avec 50 caractères tient largement dans 1 024 octets, même
// si chaque caractère est écrit en échappement JSON : 6 octets par caractère,
// 12 pour un émoji, soit 614 octets pour 50 émojis.
const maxBodyBytes = 1024

// maxNameLength est la longueur maximale d'un nom de préparateur ou de
// profil, en caractères et non en octets : « é » compte pour un caractère.
const maxNameLength = 50

// msgNotJSON est le message d'un corps qui n'est pas annoncé comme du JSON.
const msgNotJSON = "Le corps de la requête doit être du JSON, avec l'en-tête Content-Type: application/json."

// readJSON lit le corps de la requête dans v : un seul objet JSON, sans champ
// inconnu, de maxBodyBytes octets au maximum, avec l'en-tête Content-Type:
// application/json. format décrit l'objet attendu dans les messages, par
// exemple {"picker": "Nom du préparateur"}. Si le corps est refusé, readJSON
// répond 400 lui-même et renvoie false, comme parseID.
func readJSON(w http.ResponseWriter, r *http.Request, v any, format string) bool {
	// mime.ParseMediaType accepte un paramètre comme « ; charset=utf-8 », et
	// met le type en minuscules.
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, msgNotJSON)
		return false
	}

	// MaxBytesReader coupe la lecture au-delà de maxBodyBytes : un corps
	// énorme ne peut pas occuper la mémoire du serveur. DisallowUnknownFields
	// refuse un champ inconnu, au lieu de l'ignorer en silence. Limites
	// connues d'encoding/json (docs/PLAN.md, étape 7) : la comparaison des
	// noms de champs ignore la casse (« PICKER » est accepté), et une clé en
	// double garde sa dernière valeur. Sans conséquence avec un seul champ.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := decodeOne(dec, v); err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				"Le corps de la requête est vide. Format attendu : "+format+".")
		case errors.As(err, &tooLarge):
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				fmt.Sprintf("Le corps de la requête dépasse %d octets.", tooLarge.Limit))
		default:
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				"Le corps de la requête doit être un seul objet JSON de la forme "+format+", sans autre champ.")
		}
		return false
	}
	return true
}

// decodeOne lit exactement une valeur JSON dans v : après elle, le corps ne
// doit contenir que des espaces. Sans ce contrôle, json.Decoder s'arrêterait
// à la fin du premier objet et ignorerait la suite (« {...} {...} »). Un
// corps vide donne io.EOF.
func decodeOne(dec *json.Decoder, v any) error {
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Une seconde lecture doit trouver la fin du corps (io.EOF). Elle peut
	// aussi buter sur la limite de taille, ou trouver une autre valeur.
	err := dec.Decode(&struct{}{})
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("plusieurs valeurs JSON dans le corps")
}

// parseName débarrasse name de ses espaces de début et de fin, réduit à une
// seule les espaces ordinaires répétées à l'intérieur, puis le contrôle avec
// checkName. what est le complément du nom dans les messages, article compris
// : « du préparateur », « du profil ». Si le nom est refusé, parseName répond
// 400 lui-même et renvoie false, comme parseID.
func parseName(w http.ResponseWriter, name, what string) (string, bool) {
	name = strings.TrimSpace(name)
	// Une page web affiche plusieurs espaces de suite comme une seule :
	// « Jean  Pierre » et « Jean Pierre » seraient deux profils identiques à
	// l'écran. Seule l'espace ordinaire est concernée : les espaces spéciaux
	// restent refusés par checkName.
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}
	if message := checkName(name, what); message != "" {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, message)
		return "", false
	}
	return name, true
}

// checkName vérifie un nom déjà débarrassé de ses espaces de début et de fin.
// Elle renvoie le message de refus, ou une chaîne vide si le nom est accepté.
func checkName(name, what string) string {
	if name == "" {
		return "Le nom " + what + " est obligatoire."
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		return fmt.Sprintf("Le nom %s dépasse %d caractères.", what, maxNameLength)
	}
	hasLetterOrDigit := false
	for _, c := range name {
		switch {
		case c == utf8.RuneError:
			// U+FFFD : le décodeur JSON le met à la place des octets qui ne
			// sont pas de l'UTF-8 valide.
			return "Le nom " + what + " contient des octets invalides : le corps doit être encodé en UTF-8."
		case !unicode.IsPrint(c) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, c):
			// unicode.IsPrint accepte les lettres, les marques (accents), les
			// chiffres, la ponctuation, les symboles et l'espace ordinaire.
			// Il refuse les caractères de contrôle (NUL compris, que
			// PostgreSQL refuserait avec une erreur 500), les autres espaces
			// et les caractères de format, invisibles. Quelques lettres qui
			// s'affichent vides, comme le remplissage coréen U+3164, passent
			// IsPrint : Other_Default_Ignorable_Code_Point les refuse.
			return "Le nom " + what + " contient un caractère non autorisé : caractère de contrôle, espace spécial ou caractère invisible."
		case unicode.IsLetter(c) || unicode.IsNumber(c):
			hasLetterOrDigit = true
		}
	}
	// Un nom fait seulement de ponctuation, de symboles ou d'accents isolés
	// s'afficherait vide ou presque : « préparée par ... ».
	if !hasLetterOrDigit {
		return "Le nom " + what + " doit contenir au moins une lettre ou un chiffre."
	}
	return ""
}
