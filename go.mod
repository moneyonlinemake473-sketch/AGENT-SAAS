module agent-saas

go 1.22

// Dépendances à installer localement avec (voir README) :
//   go get go.mau.fi/whatsmeow
//   go get cloud.google.com/go/firestore
//   go get google.golang.org/api/sheets/v4
//   go get google.golang.org/api/option
//   go get github.com/mdp/qrterminal/v3
//   go get modernc.org/sqlite
//   go get github.com/google/uuid
//   go get golang.org/x/crypto/bcrypt
//
// Puis lancer `go mod tidy` pour figer les versions exactes.
// Ce fichier a été écrit sans accès au proxy Go modules dans cet environnement,
// donc les `require` ne sont pas encore listés ici — go mod tidy s'en charge.
