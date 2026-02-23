import os
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '3'  # This hides all TF warnings!

import numpy as np
import pandas as pd
from tensorflow import keras
from tensorflow.keras.models import Sequential
from tensorflow.keras.layers import LSTM, Dense, Dropout
from sklearn.preprocessing import MinMaxScaler
import pickle

class TrajectoryPredictor:
    def __init__(self):
        self.model = None
        self.scaler = MinMaxScaler()
        self.archetypes = {
            'rising_star': {'color': 'green', 'message': '🌟 Rising Star! Rapid improvement detected.'},
            'steady': {'color': 'blue', 'message': '📈 Steady Improver. Consistent progress.'},
            'plateauing': {'color': 'orange', 'message': '📉 Plateauing. Need intervention.'},
            'at_risk': {'color': 'red', 'message': '⚠️ At Risk. Declining performance.'}
        }
    
    def generate_synthetic_data(self, n_samples=1000):
        """
        Generate realistic athlete progression data
        """
        print("Generating synthetic training data...")
        
        all_sequences = []
        all_labels = []
        
        for _ in range(n_samples):
            archetype = np.random.choice(['rising_star', 'steady', 'plateauing', 'at_risk'])
            
            if archetype == 'rising_star':
                # Exponential growth: 4.0 -> 9.0 in 24 weeks
                base = np.linspace(4.0, 9.0, 24)
                curve = 4.0 + (9.0 - 4.0) * (1 - np.exp(-np.linspace(0, 3, 24)))
                
            elif archetype == 'steady':
                # Linear growth: 4.0 -> 7.5 in 24 weeks
                curve = np.linspace(4.0, 7.5, 24)
                
            elif archetype == 'plateauing':
                # Initial growth then plateau: 4.0 -> 6.5 -> 6.5
                curve = np.concatenate([
                    np.linspace(4.0, 6.5, 12),
                    np.ones(12) * 6.5
                ])
                
            else:  # at_risk
                # Decline: 5.0 -> 3.0
                curve = np.linspace(5.0, 3.0, 24)
            
            # Add realistic noise
            noise = np.random.normal(0, 0.3, 24)
            curve = np.clip(curve + noise, 0, 10)
            
            # Create sequences: use 5 weeks to predict 6th
            for i in range(len(curve) - 6):
                seq = curve[i:i+5]
                label = curve[i+5]
                all_sequences.append(seq)
                all_labels.append(label)
        
        return np.array(all_sequences), np.array(all_labels)
    
    def build_and_train_model(self):
        """
        Build and train LSTM model
        """
        print("Building LSTM model...")
        
        # Generate data
        X, y = self.generate_synthetic_data(n_samples=1000)
        
        # Reshape for LSTM [samples, timesteps, features]
        X = X.reshape((X.shape[0], X.shape[1], 1))
        
        # Normalize
        X = self.scaler.fit_transform(X.reshape(-1, 1)).reshape(X.shape)
        y = y.reshape(-1, 1)
        y = self.scaler.transform(y).flatten()
        
        # Build model
        model = Sequential([
            LSTM(64, activation='relu', return_sequences=True, input_shape=(5, 1)),
            Dropout(0.2),
            LSTM(32, activation='relu'),
            Dropout(0.2),
            Dense(16, activation='relu'),
            Dense(1)
        ])
        
        model.compile(optimizer='adam', loss='mse', metrics=['mae'])
        
        print("Training model...")
        model.fit(X, y, epochs=50, batch_size=32, verbose=0, validation_split=0.2)
        
        self.model = model
        print("✅ Model trained successfully!")
        
        # Save model
        model.save('trajectory_model.h5')
        pickle.dump(self.scaler, open('scaler.pkl', 'wb'))
    
    def predict_trajectory(self, recent_scores):
        """
        Predict next 6 months (24 weeks) of performance
        
        Args:
            recent_scores: List of last 5 technique scores [6.5, 7.0, 7.3, 7.5, 7.8]
        
        Returns:
            predictions, archetype, time_to_pro
        """
        if self.model is None:
            print("Loading model...")
            # We add compile=False to bypass the version bug!
            self.model = keras.models.load_model('trajectory_model.h5', compile=False) 
            self.scaler = pickle.load(open('scaler.pkl', 'rb'))
        
        # Normalize input
        recent_scores = np.array(recent_scores).reshape(-1, 1)
        recent_scores_norm = self.scaler.transform(recent_scores)
        
        # Predict next 24 weeks
        predictions = []
        current_sequence = recent_scores_norm.flatten().tolist()
        
        for _ in range(24):
            X_pred = np.array(current_sequence[-5:]).reshape(1, 5, 1)
            next_score = self.model.predict(X_pred, verbose=0)[0][0]
            predictions.append(next_score)
            current_sequence.append(next_score)
        
        # Denormalize
        predictions = self.scaler.inverse_transform(np.array(predictions).reshape(-1, 1)).flatten()
        
        # Classify archetype
        archetype = self.classify_archetype(recent_scores.flatten(), predictions)
        
        # Calculate time to pro level (assuming pro = 8.5)
        time_to_pro = self.estimate_time_to_pro(predictions, pro_threshold=8.5)
        
        return predictions, archetype, time_to_pro
    
    def classify_archetype(self, history, predictions):
        """
        Classify trajectory type
        """
        # Calculate improvement rate
        recent_trend = np.mean(np.diff(history[-3:]))
        future_trend = np.mean(np.diff(predictions[:12]))
        
        if future_trend > 0.15:
            return 'rising_star'
        elif future_trend > 0.05:
            return 'steady'
        elif future_trend > -0.05:
            return 'plateauing'
        else:
            return 'at_risk'
    
    def estimate_time_to_pro(self, predictions, pro_threshold=8.5):
        """
        Estimate weeks to reach pro level
        """
        for i, score in enumerate(predictions):
            if score >= pro_threshold:
                return i + 1  # weeks
        return None  # Won't reach pro in predicted timeframe
    
    def generate_report(self, recent_scores, current_score):
        """
        Generate complete trajectory report
        """
        # THE PRO BYPASS: If they are already elite, skip the LSTM and give them their flowers!
        if current_score >= 8.5:
            return {
                'current_score': float(current_score),
                'predicted_scores': [float(current_score)] * 24, # Flat line at the top
                'archetype': 'elite',
                'archetype_message': '🌟 Elite Performer. Maintaining peak level.',
                'time_to_pro_weeks': 0,
                'improvement_rate': 0.0,
                'recommendation': "You are already performing at a professional level! Focus on consistency, injury prevention, and marginal gains to maintain this peak."
            }
            
        # ... rest of the original generate_report code stays exactly the same ...
        if len(recent_scores) == 1:
            # Generate realistic history leading to current score
            start_score = max(3.0, current_score - 2.0)
            recent_scores = list(np.linspace(start_score, current_score, 5))
        
        predictions, archetype, time_to_pro = self.predict_trajectory(recent_scores)
        
        archetype_info = self.archetypes[archetype]
        
        report = {
            'current_score': recent_scores[-1],
            'predicted_scores': predictions.tolist(),
            'archetype': archetype,
            'archetype_message': archetype_info['message'],
            'time_to_pro_weeks': time_to_pro,
            'improvement_rate': np.mean(np.diff(recent_scores[-3:])) if len(recent_scores) >= 3 else 0,
            'recommendation': self.get_recommendation(archetype, time_to_pro)
        }
        
        return report
    
    def get_recommendation(self, archetype, time_to_pro):
        """
        Personalized recommendations based on trajectory
        """
        if archetype == 'rising_star':
            return "Excellent progress! Maintain current training intensity. You're on track for elite level."
        elif archetype == 'steady':
            return f"Consistent improvement detected. {'Estimated ' + str(time_to_pro) + ' weeks to pro level' if time_to_pro else 'Keep up the good work'}."
        elif archetype == 'plateauing':
            return "Performance plateau detected. Consider: 1) Increase training variety, 2) Work with a coach, 3) Focus on weak areas identified in analysis."
        else:  # at_risk
            return "⚠️ Declining performance. Immediate intervention needed: Check for injuries, review training load, consider rest period."

# Test code
if __name__ == "__main__":
    predictor = TrajectoryPredictor()
    
    # Train model
    predictor.build_and_train_model()
    
    # Test prediction
    recent_scores = [5.0, 5.8, 6.5, 7.2, 7.8]
    report = predictor.generate_report(recent_scores, current_score=7.8)
    
    print("\n" + "="*50)
    print("TRAJECTORY PREDICTION REPORT")
    print("="*50)
    print(f"Current Score: {report['current_score']:.1f}")
    print(f"Archetype: {report['archetype_message']}")
    print(f"Time to Pro: {report['time_to_pro_weeks']} weeks" if report['time_to_pro_weeks'] else "Time to Pro: Not in forecast")
    print(f"\nRecommendation:\n{report['recommendation']}")
    print("="*50)