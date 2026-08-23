/**
 * À coller dans l'éditeur Apps Script (script.google.com) d'UN classeur "hub"
 * qui a accès à tous les classeurs de tes clients, OU à coller individuellement
 * dans chaque classeur client (plus simple à mettre en place au début).
 *
 * Installation (par classeur client, le plus simple pour démarrer) :
 * 1. Ouvre le Google Sheet du client
 * 2. Extensions > Apps Script
 * 3. Colle ce code, sauvegarde
 * 4. Dans le menu horloge à gauche (Déclencheurs) > Ajouter un déclencheur
 *    - Fonction: clearConversationDaily
 *    - Type d'événement: Basé sur le temps > Minuterie jour > minuit à 1h
 * 5. Autorise le script (première fois seulement)
 *
 * Alternative plus scalable pour 60 clients : un seul script "hub" qui boucle
 * sur une liste d'IDs de classeurs (stockée dans une feuille de config) et
 * appelle SpreadsheetApp.openById(id) pour chacun. Demande-moi si tu veux
 * cette version, je te la prépare.
 */
function clearConversationDaily() {
  const sheet = SpreadsheetApp.getActiveSpreadsheet().getSheetByName('Conversation');
  if (!sheet) return;

  const lastRow = sheet.getLastRow();
  if (lastRow > 1) {
    // On garde la ligne d'en-tête (ligne 1), on vide tout le reste
    sheet.getRange(2, 1, lastRow - 1, 3).clearContent();
  }
}
