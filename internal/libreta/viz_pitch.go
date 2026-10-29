package libreta

// Costanti della larghezza della MRZ dal passo dei caratteri (vedi viz_geom.go). Gli estremi orizzontali delle due righe
// si sporcano nelle scansioni reali (bordo del libro, ombre, plastificazione); il passo dei caratteri invece e' periodico
// e fisso per norma ICAO (44 caratteri a 2,54 mm), e l'inchiostro di una riga e' vizMRZChars passi.

// vizMRZChars: larghezza dell'inchiostro di una riga MRZ in passi di carattere (43 passi piu' un glifo di circa 0,5
// passi: misurato su righe pulite di sette scansioni, 0,5% di scarto).
const vizMRZChars = 43.5

// vizPitchTol: la larghezza misurata dagli estremi si corregge con quella dal passo se la supera di piu' di questa frazione.
const vizPitchTol = 1.025
