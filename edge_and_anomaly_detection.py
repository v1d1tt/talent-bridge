import cv2
import numpy as np
import pandas as pd
import os
import glob
from dotenv import load_dotenv
from google import genai

# NEW: Import your trained Trajectory Predictor
from trajectory_predictor import TrajectoryPredictor

# Load environment variables
load_dotenv()

# Configure Gemini API
client = genai.Client(api_key=os.getenv('GEMINI_API_KEY'))

class EdgeDetector:
    def __init__(self):
        print("Edge And Anomaly Detector initialized!")
        # NEW: Initialize the LSTM model
        self.trajectory_predictor = TrajectoryPredictor()
    
    def calculate_angle(self, a, b, c):
        """Calculate angle between three points"""
        a = np.array(a)
        b = np.array(b)
        c = np.array(c)
        
        radians = np.arctan2(c[1]-b[1], c[0]-b[0]) - np.arctan2(a[1]-b[1], a[0]-b[0])
        angle = np.abs(radians * 180.0 / np.pi)
        
        if angle > 180.0:
            angle = 360 - angle
            
        return angle
    
    def detect_contact_moment(self, video_path):
        """
        Detect the moment of ball-bat/racket contact using edge detection
        """
        print(f"Analyzing video: {video_path}")
        
        cap = cv2.VideoCapture(video_path)
        contact_frames = []
        frame_count = 0
        prev_frame = None
        
        while cap.isOpened():
            ret, frame = cap.read()
            if not ret:
                break
            
            gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
            edges = cv2.Canny(gray, 50, 150)
            
            if prev_frame is not None:
                frame_diff = cv2.absdiff(prev_frame, gray)
                motion_intensity = np.sum(frame_diff)
                
                if motion_intensity > 1000000:
                    contact_frames.append({
                        'frame_number': frame_count,
                        'intensity': motion_intensity
                    })
            
            prev_frame = gray
            frame_count += 1
        
        cap.release()
        
        if contact_frames:
            contact = max(contact_frames, key=lambda x: x['intensity'])
            print(f"Contact detected at frame: {contact['frame_number']}")
            return contact
        else:
            print("No clear contact detected")
            return None
    
    def build_rag_database(self, sport_type='cricket', reference_folder='reference_data'):
        """
        RAG Step 1: Read Pro CSVs based on the specific sport!
        """
        import os, glob
        target_folder = os.path.join(reference_folder, sport_type)
        print(f"Building {sport_type.upper()} RAG database from {target_folder}...")
        
        pro_angles = []
        csv_files = glob.glob(os.path.join(target_folder, '*.csv'))
        
        if not csv_files:
            print("No reference CSVs found! Using default fallback.")
            return {'mean_angle': 150.0, 'std_angle': 5.0}
            
        for file in csv_files:
            try:
                df = pd.read_csv(file)
                for index, row in df.iterrows():
                    right_shoulder = np.array([row['right_shoulder_x'], row['right_shoulder_y']])
                    right_elbow = np.array([row['right_elbow_x'], row['right_elbow_y']])
                    right_wrist = np.array([row['right_wrist_x'], row['right_wrist_y']])
                    
                    angle = self.calculate_angle(right_shoulder, right_elbow, right_wrist)
                    pro_angles.append(angle)
            except Exception as e:
                print(f"Skipping {file}: {e}")
                
        mean_angle = np.mean(pro_angles)
        std_angle = max(np.std(pro_angles), 1.0)
        print(f"RAG Database Built! Pro Mean Angle: {mean_angle:.1f}°, STD: {std_angle:.1f}°")
        
        return {'mean_angle': mean_angle, 'std_angle': std_angle}

    def score_technique(self, pose_csv_path, contact_data, pro_baseline):
        """
        Anomaly Detection: Compare amateur against the RAG Pro Baseline
        """
        try:
            df = pd.read_csv(pose_csv_path)
            angles = []
            
            for index, row in df.iterrows():
                right_shoulder = np.array([row['right_shoulder_x'], row['right_shoulder_y']])
                right_elbow = np.array([row['right_elbow_x'], row['right_elbow_y']])
                right_wrist = np.array([row['right_wrist_x'], row['right_wrist_y']])
                angles.append(self.calculate_angle(right_shoulder, right_elbow, right_wrist))
                    
            avg_angle = np.mean(angles)
            
            z_score = abs(avg_angle - pro_baseline['mean_angle']) / pro_baseline['std_angle']
            is_anomaly = z_score > 2.0 
            
            technique_score = 10.0 - (z_score * 1.5)
            technique_score = max(0, min(10, technique_score))
            
            return {
                'technique_score': round(technique_score, 1),
                'contact_quality': 8.0 if contact_data else 5.0,
                'avg_elbow_angle': round(avg_angle, 1),
                'is_injury_risk': bool(is_anomaly),
                'pro_ideal_angle': round(pro_baseline['mean_angle'], 1)
            }
        except Exception as e:
            print(f"Error: {e}")
            return None
    
    def get_ai_feedback(self, scores):
        """
        Generate personalized feedback using Gemini + RAG Data
        """
        prompt = f"""
        You are a professional sports coach. Analyze this athlete's performance against professional baselines:

        Technique Score: {scores['technique_score']}/10
        Contact Quality: {scores['contact_quality']}/10
        Amateur Elbow Angle: {scores['avg_elbow_angle']}°
        Professional Ideal Angle: {scores.get('pro_ideal_angle', 'N/A')}°
        Injury Risk Flag: {'Yes (Anomaly)' if scores.get('is_injury_risk') else 'No'}

        Provide brief, encouraging feedback in 2-3 lines:
        - What they did well
        - One thing to improve (specifically mention how they compare to the professional ideal angle)
        - A quick actionable tip

        Keep it motivating and specific.
        """
        
        try:
            response = client.models.generate_content(
                model='gemini-2.5-flash',
                contents=prompt
            )
            return response.text
        except Exception as e:
            print(f"Gemini error: {e}")
            return "Good effort! Try to match the professional ideal angle more closely."
    
    def analyze_complete(self, video_path, pose_csv_path, sport_type='cricket'):
        print("=" * 50)
        print(f"Starting Complete Analysis for: {sport_type.upper()}")
        print("=" * 50)
        
        # 1. RAG Retrieval for the specific sport
        pro_baseline = self.build_rag_database(sport_type)
        
        # 2. Edge Detection
        contact_data = self.detect_contact_moment(video_path)
        
        # 3. Anomaly Detection & Scoring
        scores = self.score_technique(pose_csv_path, contact_data, pro_baseline)
        
        if scores:
            # 4. Get the AI feedback
            scores['feedback'] = self.get_ai_feedback(scores)
            
            # 5. NEW: Trajectory Prediction!
            current_score = scores['technique_score']
            trajectory_report = self.trajectory_predictor.generate_report(
                recent_scores=[current_score],  # Simulate history based on today's score
                current_score=current_score
            )
            # Add trajectory to the final dictionary for the FastAPI team
            scores['trajectory'] = trajectory_report
            
            # Print ALL results
            print("\n" + "=" * 50)
            print("🏏 IMMEDIATE TECHNIQUE ANALYSIS")
            print("=" * 50)
            print(f"Technique Score: {scores['technique_score']}/10")
            print(f"Amateur Angle: {scores['avg_elbow_angle']}° vs Pro Ideal: {scores['pro_ideal_angle']}°")
            if scores['is_injury_risk']:
                print("🚨 ANOMALY DETECTED: High Injury Risk! Form deviates heavily from pros.")
            
            print(f"\n🧠 AI Coach Feedback:\n{scores['feedback']}")
            
            print("\n" + "=" * 50)
            print("🚀 LONG-TERM TRAJECTORY FORECAST (LSTM)")
            print("=" * 50)
            print(f"Athlete Type: {trajectory_report['archetype_message']}")
            
            # --- THE FIX: Explicitly checking for 0 ---
            if trajectory_report['time_to_pro_weeks'] == 0:
                print("⏱️ Time to Pro Level: 0 weeks (Already at Elite Pro Level!)")
            elif trajectory_report['time_to_pro_weeks'] is not None:
                print(f"⏱️ Time to Pro Level: {trajectory_report['time_to_pro_weeks']} weeks")
            else:
                print("⏱️ Time to Pro Level: Not achievable in forecast period")
                
            print(f"\n📊 Recommendation:\n{trajectory_report['recommendation']}")
            print("=" * 50)
            
        return scores