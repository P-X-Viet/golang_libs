func revokeTokenHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JWTID string `json:"jwt_id"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Revoke the token
	err := revokeJWT(req.JWTID)
	if err != nil {
		http.Error(w, "Failed to revoke token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Token revoked successfully",
	})
}